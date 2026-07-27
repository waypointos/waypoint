package natshub

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

const startTimeout = 5 * time.Second

// Config holds options for starting the embedded NATS hub.
type Config struct {
	Operator    *operator.Operator
	ClientPort  int
	LeafWSPort  int
	HTTPMonPort int

	// Logs enables nats-server's stdout/stderr logging. Tests leave this
	// false to keep output quiet; production should set it true so leaf
	// dial failures, retries, and auth rejections are visible in the
	// process log (journalctl on Railway, etc.).
	Logs bool
}

// Hub wraps the embedded nats-server and provides Waypoint-specific helpers.
type Hub struct {
	cfg    Config
	Server *server.Server
}

// Start creates and starts an embedded NATS server configured for operator-mode
// JWT authentication. Accounts can be registered at runtime via RegisterAccount.
func Start(_ context.Context, cfg Config) (*Hub, error) {
	opts := &server.Options{
		ServerName: "waypoint-proxy",

		Port:        cfg.ClientPort,
		TrustedKeys: []string{cfg.Operator.PublicKey()},

		AccountResolver: &server.MemAccResolver{},

		Websocket: server.WebsocketOpts{
			Port: cfg.LeafWSPort,
			// NoTLS=true is intentional: this listener binds to loopback
			// inside the container, and TLS is terminated upstream at
			// Cloudflare / the Railway edge before traffic reaches the
			// Go HTTP server's /leaf reverse proxy. nats-server has no
			// visibility into that and logs a generic
			//   [WRN] Websocket not configured with TLS. DO NOT USE IN PRODUCTION!
			// on startup — expected for this topology, not a regression.
			NoTLS: true,
		},

		LeafNode: server.LeafNodeOpts{
			Port: -1,
		},

		HTTPPort: cfg.HTTPMonPort,

		// Basemap tiles travel over request/reply; the 1 MiB default is tight
		// for dense low-zoom vector tiles, so allow up to 4 MiB.
		MaxPayload: 4 * 1024 * 1024,
	}

	s, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}
	// nats-server's NewServer leaves the logger nil; without an explicit
	// ConfigureLogger() call, every log message is silently dropped even
	// when NoLog=false. The library only auto-configures the logger on
	// config reload (server/reload.go), not at startup, so embedded
	// callers must opt in. Production passes Logs=true so leaf-node
	// lifecycle events reach Railway logs; tests leave it false.
	if cfg.Logs {
		s.ConfigureLogger()
	}
	go s.Start()
	if !s.ReadyForConnections(startTimeout) {
		s.Shutdown()
		return nil, fmt.Errorf("nats server not ready within %s", startTimeout)
	}
	return &Hub{cfg: cfg, Server: s}, nil
}

// Shutdown stops the embedded NATS server.
func (h *Hub) Shutdown() {
	if h.Server != nil {
		h.Server.Shutdown()
	}
}

// LeafURL returns the WebSocket URL leaf-node clients should connect to.
func (h *Hub) LeafURL() string {
	if h.Server == nil {
		return ""
	}
	return h.Server.WebsocketURL()
}

// LeafHandler returns an HTTP handler that reverse-proxies WebSocket
// upgrades to the embedded nats-server's local WebSocket port. Both
// servers run in the same process, so the "remote" is loopback.
//
// Path forwarding: the path on the request reaching this handler is
// passed through to nats-server verbatim. nats-server's leaf-over-WS
// protocol uses a "/leafnode" path suffix, so a leaf remote configured
// with url=wss://host/leaf will actually dial /leaf/leafnode. The
// caller is expected to wrap this handler with http.StripPrefix to
// chop the public mount path ("/leaf") so what reaches nats-server is
// just "/leafnode":
//
//	rt.Public("GET /leaf/", http.StripPrefix("/leaf", hub.LeafHandler()))
//
// Auth happens at the NATS layer — nats-server validates the agent's
// account JWT and user seed against the operator trust key (see
// TrustedKeys above). This handler is deliberately not gated by
// HTTP-session auth, because rover credentials are NATS credentials,
// not WorkOS sessions.
func (h *Hub) LeafHandler() http.Handler {
	if h.Server == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "hub not running", http.StatusServiceUnavailable)
		})
	}
	u, err := url.Parse(h.LeafURL())
	if err != nil || u.Host == "" {
		// LeafURL comes straight from nats-server; an unparseable value
		// would be a bug, not a runtime condition.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "invalid leaf url", http.StatusInternalServerError)
		})
	}
	// httputil.ReverseProxy dials with HTTP semantics; the WebSocket
	// Upgrade headers on the incoming request promote the connection
	// during the handshake. Scheme must be http (not ws) for the dial.
	target := &url.URL{Scheme: "http", Host: u.Host}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// SetURL fills in Scheme/Host from target and joins paths;
			// with target.Path empty, the request's existing Path (after
			// the caller's StripPrefix) is preserved as-is.
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.SetXForwarded()
		},
	}
}

// RegisterAccount stores the JWT in the resolver and, if the account is
// already loaded in memory, pushes the new claims so imports/exports/perms
// take effect without restarting the hub. Without the second step a re-mint
// (e.g. RefreshSessionsAccount after an enroll) only affects future connects.
func (h *Hub) RegisterAccount(accountJWT string) error {
	claims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return fmt.Errorf("decode account jwt: %w", err)
	}
	accountPubKey := claims.Subject
	if err := h.Server.AccountResolver().Store(accountPubKey, accountJWT); err != nil {
		return err
	}
	if acc, _ := h.Server.LookupAccount(accountPubKey); acc != nil {
		h.Server.UpdateAccountClaims(acc, claims)
	}
	return nil
}

// RevokeAccount overwrites the stored JWT for the given account with a revoked
// (expired, no-permission) variant and forcibly disconnects every client that
// is currently authenticated under that account. The caller is responsible for
// providing a revoked JWT produced by Operator.MintRevokedAccountJWT.
//
// New connections fail authentication because the JWT is expired. Any active
// connection is dropped two ways belt-and-braces: UpdateAccountClaims rebuilds
// the in-memory account permissions to deny-all (so any subsequent pub/sub is
// rejected), and DisconnectClientByID closes the socket immediately.
func (h *Hub) RevokeAccount(accountPubKey, revokedJWT string) error {
	claims, err := jwt.DecodeAccountClaims(revokedJWT)
	if err != nil {
		return fmt.Errorf("decode revoked jwt: %w", err)
	}
	if claims.Subject != accountPubKey {
		return fmt.Errorf("revoked jwt subject %q does not match account %q", claims.Subject, accountPubKey)
	}

	if err := h.Server.AccountResolver().Store(accountPubKey, revokedJWT); err != nil {
		return fmt.Errorf("store revoked jwt: %w", err)
	}

	// If the account is currently loaded, push the new (empty) claims so that
	// any in-flight permissions check denies. LookupAccount returns nil
	// without error when the account isn't loaded yet — that's fine.
	if acc, _ := h.Server.LookupAccount(accountPubKey); acc != nil {
		h.Server.UpdateAccountClaims(acc, claims)
	}

	// Kick any currently-authenticated clients. We pass Username:true so the
	// returned ConnInfo.Account field is populated (nats-server only fills it
	// when the auth section is requested), even though the Account filter is
	// applied server-side regardless.
	connz, err := h.Server.Connz(&server.ConnzOptions{
		Account:  accountPubKey,
		Username: true,
		Limit:    1024,
	})
	if err != nil {
		return fmt.Errorf("list connections: %w", err)
	}
	for _, c := range connz.Conns {
		_ = h.Server.DisconnectClientByID(c.Cid)
	}
	return nil
}
