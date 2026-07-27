// agent/internal/gateway/ws.go
//
// Browser ↔ NATS WebSocket bridge. JSON envelopes:
//
//	{ "action": "sub",   "subject": "waypoint.<id>.telemetry.drive" }
//	{ "action": "unsub", "subject": "waypoint.<id>.telemetry.drive" }
//	{ "action": "pub",   "subject": "waypoint.<id>.cmd.drive", "data": "<base64>" }
//
// Inbound: payloads are base64-encoded protobuf bytes.
// Outbound: server pushes the same shape when subscribed subjects receive messages.
package gateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	nats "github.com/nats-io/nats.go"

	"github.com/waypointos/waypoint/agent/internal/magiclink"
	"github.com/waypointos/waypoint/agent/internal/session"
)

type envelope struct {
	Action  string          `json:"action"`
	Subject string          `json:"subject"`
	Data    string          `json:"data,omitempty"`
	Reply   string          `json:"reply,omitempty"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

type Gateway struct {
	natsURL      string
	roverID      string
	token        string
	operatorPub  string
	localIss     *session.LocalIssuer
	sessions     *sessionStore
	upgrader     websocket.Upgrader
	moduleStatic *ModuleStaticHandler
	moduleProxy  *ModuleProxyHandler
	localModules LocalModules
	episodes     EpisodeStore
}

// New constructs a Gateway. operatorPub is the NKey public key of the proxy operator;
// when empty, magic-link login is disabled and /login returns 503.
// localIss signs session user JWTs handed to the dashboard via
// POST /api/auth/nats-cred; when nil, that endpoint returns 503.
// modReg is the shared module proxy registry (populated by modules.Reconciler);
// when nil, an empty one is allocated so /module/* handlers never panic.
// moduleMountRoot is where the agent mounts module images read-only for static
// asset serving (matches modules.DefaultModuleMountRoot).
func New(natsURL, roverID, bootstrapToken, operatorPub string, localIss *session.LocalIssuer, modReg *ModuleProxyRegistry, moduleMountRoot string, localModules LocalModules) *Gateway {
	if moduleMountRoot == "" {
		moduleMountRoot = "/run/waypoint/module-root"
	}
	if modReg == nil {
		modReg = NewModuleProxyRegistry()
	}
	return &Gateway{
		natsURL:      natsURL,
		roverID:      roverID,
		token:        bootstrapToken,
		operatorPub:  operatorPub,
		localIss:     localIss,
		sessions:     newSessionStore(localSessionCookieTTL),
		moduleStatic: NewModuleStaticHandler(moduleMountRoot),
		moduleProxy:  NewModuleProxyHandler(modReg),
		localModules: localModules,
		upgrader: websocket.Upgrader{
			// Accept same-origin/non-browser dials but reject foreign web
			// origins, so a DNS-rebinding page can't ride the auto-issued
			// cookie to drive the rover. See isLocalHost.
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					// Non-browser client (CLI/native). Scripted browser WS
					// always sends Origin, so this can't be a rebinding bypass.
					return true
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return isLocalHost(u.Host)
			},
		},
	}
}

func (g *Gateway) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", g.handleWS)
	mux.HandleFunc("GET /login", g.handleLogin)
	mux.HandleFunc("GET /api/me", g.HandleMe)
	mux.HandleFunc("POST /api/auth/nats-cred", g.handleNatsCred)
	mux.HandleFunc("GET /api/modules", g.handleModulesList)
	mux.HandleFunc("POST /api/modules/stage", g.handleModuleStage)
	mux.HandleFunc("POST /api/modules/confirm", g.handleModuleConfirm)
	mux.HandleFunc("DELETE /api/modules/{id}", g.handleModuleUninstall)
	mux.HandleFunc("GET /api/episodes", g.handleEpisodesList)
	mux.HandleFunc("GET /api/episodes/{id}/download", g.handleEpisodeDownload)
	mux.HandleFunc("DELETE /api/episodes/{id}", g.handleEpisodeDelete)
	mux.Handle("/module/", g.moduleRouter())
	return mux
}

// moduleRouter dispatches /module/<id>/static/* to the static handler and
// everything else under /module/<id>/* to the reverse-proxy handler.
func (g *Gateway) moduleRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/static/") {
			g.moduleStatic.ServeHTTP(w, r)
			return
		}
		g.moduleProxy.ServeHTTP(w, r)
	})
}

// handleNatsCred mints a short-lived NATS session JWT for the dashboard,
// mirroring the proxy's POST /api/auth/nats-cred shape so the dashboard's
// bus client can hand (jwt, seed) to a NATS connection regardless of which
// mode it's in. Authorization mirrors /ws: bootstrap token via the
// `wp_local_token` cookie OR a `?token=` query param.
//
// The agent's embedded NATS doesn't run in operator mode, so the local
// broker doesn't enforce this JWT: it is advisory on local-only setups
// and becomes mandatory only if the local NATS flips to operator mode.
func (g *Gateway) handleNatsCred(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if g.localIss == nil {
		http.Error(w, "local issuer not configured", http.StatusServiceUnavailable)
		return
	}

	tok, seed, err := g.localIss.MintLocalSession(g.roverID, session.LocalSessionTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jwt":  tok,
		"seed": base64.StdEncoding.EncodeToString(seed),
		// Unix seconds; matches proxy's response shape so the dashboard
		// can use the same refresh logic for both backends.
		"expiresAt": time.Now().Add(session.LocalSessionTTL).Unix(),
		// Agent doesn't expose a separate NATS WebSocket — the dashboard
		// already speaks to /ws (the JSON envelope bridge). Surfacing
		// /ws here keeps the response shape consistent with the proxy
		// (which returns "/ws/nats") even though local-mode dashboards
		// don't currently use the JWT to authenticate that socket.
		"natsUrl": "/ws",
	})
}

// localAuthorized gates /ws and /api/auth/nats-cred. It requires a local peer
// IP (loopback / RFC1918 / link-local / Tailscale-CGNAT) and then either a
// `?token=` matching the bootstrap admin token (the off-LAN URL override) or a
// valid opaque session in the `wp_local_token` cookie. The cookie never carries
// the bootstrap token itself — see sessionStore.
func (g *Gateway) localAuthorized(r *http.Request) bool {
	if g.token == "" {
		return false
	}
	if !isLocalAddr(r.RemoteAddr) {
		return false
	}
	if r.URL.Query().Get("token") == g.token {
		return true
	}
	if c, err := r.Cookie("wp_local_token"); err == nil && g.sessions.valid(c.Value) {
		return true
	}
	return false
}

// HandleMe returns the local-mode identity. The proxy serves its own /api/me
// with WorkOS session data; the agent always reports mode=local + isAdmin=true.
func (g *Gateway) HandleMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"mode":"local","isAdmin":true,"roverId":"` + g.roverID + `"}`))
}

// WithLocalSession wraps the SPA handler so a LAN visitor is authenticated
// automatically: on a document load it issues an opaque wp_local_token session
// cookie that /ws and /api/auth/nats-cred accept. The spec treats the LAN as
// trusted (§6.6), so issuance is guarded by a configured bootstrap token plus
// two checks: a local Host (DNS-rebinding defense — a foreign page rebinding to
// the rover's IP sends its own public Host) and a local peer IP (off-LAN
// exposure defense). The cookie is set on navigations (Accept: text/html), not
// on static assets, and only when there isn't already a valid session.
func (g *Gateway) WithLocalSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.shouldIssueSession(r) {
			g.setSessionCookie(w)
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Gateway) shouldIssueSession(r *http.Request) bool {
	if g.token == "" || !isLocalHost(r.Host) || !isLocalAddr(r.RemoteAddr) {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return false
	}
	// Don't re-mint when the client already holds a valid session.
	if c, err := r.Cookie("wp_local_token"); err == nil && g.sessions.valid(c.Value) {
		return false
	}
	return true
}

// setSessionCookie mints a fresh opaque session and sets it as the
// wp_local_token cookie. Shared by the auto-issue path and the magic-link login.
func (g *Gateway) setSessionCookie(w http.ResponseWriter) {
	tok, err := g.sessions.issue()
	if err != nil {
		slog.Warn(fmt.Sprintf("local session: issue: %v", err))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "wp_local_token",
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(localSessionCookieTTL / time.Second),
	})
}

// isLocalHost reports whether a Host/Origin authority (with or without a port)
// is a trusted local target: localhost, an mDNS ".local" name, or a private,
// loopback, link-local, or Tailscale-CGNAT (100.64.0.0/10) IP literal. Public
// names (e.g. a DNS-rebinding attacker's domain) and public IPs are rejected.
func isLocalHost(authority string) bool {
	host := authority
	if h, _, err := net.SplitHostPort(authority); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")                       // bare IPv6 literal without a port
	host = strings.ToLower(strings.TrimSuffix(host, ".")) // case-insensitive; tolerate FQDN trailing dot
	if host == "localhost" {
		return true
	}
	// Single-label mDNS name only (e.g. "waypoint.local"); reject "evil.com.local".
	if label, ok := strings.CutSuffix(host, ".local"); ok {
		return label != "" && !strings.Contains(label, ".")
	}
	return isLocalIP(net.ParseIP(host))
}

// isLocalAddr reports whether a net/http RemoteAddr ("ip:port") is a local
// peer (loopback / RFC1918 / link-local / Tailscale-CGNAT).
func isLocalAddr(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	return isLocalIP(net.ParseIP(host))
}

// isLocalIP classifies an IP as a trusted local target: loopback, RFC1918
// private, link-local, or Tailscale-CGNAT (100.64.0.0/10). nil → false.
func isLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

func (g *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	if g.operatorPub == "" {
		http.Error(w, "magic-link login disabled (no [proxy] in identity.toml)", http.StatusServiceUnavailable)
		return
	}
	tok := r.URL.Query().Get("token")
	if tok == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}
	if _, err := magiclink.Verify(g.operatorPub, tok); err != nil {
		http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
		return
	}
	// LAN is trusted: a valid magic link grants full local privileges via an
	// opaque session cookie (never the bootstrap token itself).
	g.setSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (g *Gateway) handleWS(w http.ResponseWriter, r *http.Request) {
	if !g.localAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	c, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn(fmt.Sprintf("ws upgrade: %v", err))
		return
	}
	defer c.Close()

	// No-auth connect: mapped to NoAuthUser ("_default") on the embedded
	// server. Module-issued creds are not needed here; broad permissions
	// match historical behavior. See agent/internal/localauth.
	nc, err := nats.Connect(g.natsURL)
	if err != nil {
		slog.Warn(fmt.Sprintf("ws → nats connect: %v", err))
		return
	}
	defer nc.Close()

	subs := struct {
		sync.Mutex
		m map[string]*nats.Subscription
	}{m: map[string]*nats.Subscription{}}

	// writer pump: serialise writes through a single goroutine.
	out := make(chan envelope, 64)
	done := make(chan struct{})
	go func() {
		for env := range out {
			if err := c.WriteJSON(env); err != nil {
				close(done)
				return
			}
		}
	}()

	defer func() {
		subs.Lock()
		for _, s := range subs.m {
			_ = s.Unsubscribe()
		}
		subs.Unlock()
		close(out)
	}()

	for {
		var env envelope
		if err := c.ReadJSON(&env); err != nil {
			return
		}
		if !g.subjectAllowed(env.Subject) {
			continue
		}
		switch env.Action {
		case "sub":
			subs.Lock()
			if _, ok := subs.m[env.Subject]; ok {
				subs.Unlock()
				continue
			}
			subject := env.Subject
			s, err := nc.Subscribe(subject, func(m *nats.Msg) {
				select {
				case out <- envelope{
					Action: "msg",
					// Use the concrete delivered subject, not the subscription pattern (which may be a wildcard).
					Subject: m.Subject,
					Data:    base64.StdEncoding.EncodeToString(m.Data),
				}:
				case <-done:
				}
			})
			if err == nil {
				subs.m[subject] = s
			}
			subs.Unlock()
		case "unsub":
			subs.Lock()
			if s, ok := subs.m[env.Subject]; ok {
				_ = s.Unsubscribe()
				delete(subs.m, env.Subject)
			}
			subs.Unlock()
		case "pub":
			raw, err := base64.StdEncoding.DecodeString(env.Data)
			if err != nil {
				continue
			}
			_ = nc.Publish(env.Subject, raw)
		}
	}
}

// subjectAllowed restricts the bridge to this rover's namespace.
func (g *Gateway) subjectAllowed(subject string) bool {
	return strings.HasPrefix(subject, "waypoint."+g.roverID+".")
}
