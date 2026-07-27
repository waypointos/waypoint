package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/jwt/v2"
	nats "github.com/nats-io/nats.go"

	natssrv "github.com/waypointos/waypoint/agent/internal/nats"
	"github.com/waypointos/waypoint/agent/internal/session"
)

// helper: start an embedded NATS server.
func newNATS(t *testing.T) *natssrv.Server {
	t.Helper()
	srv, err := natssrv.StartEmbedded(natssrv.Options{Port: -1})
	if err != nil {
		t.Fatalf("nats start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatalf("nats ready: %v", err)
	}
	t.Cleanup(srv.Shutdown)
	return srv
}

func TestGateway_pubsubRoundtrip(t *testing.T) {
	ns := newNATS(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", nil, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	wsURL := strings.Replace(httpsrv.URL, "http://", "ws://", 1) + "/ws?token=tok"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	// Subscribe to telemetry.drive
	must(t, c.WriteJSON(envelope{Action: "sub", Subject: "waypoint.rover-01.telemetry.drive"}))

	// Publish a frame to the same subject from the other side via direct NATS.
	// The gateway handles the sub envelope and calls nc.Subscribe asynchronously,
	// so a single publish can race that registration and be dropped (fire-and-
	// forget pub-sub has no replay). Republish on a tick until the gateway's
	// subscription is live and our message bounces back through the WS.
	// No-auth connect: mapped to NoAuthUser ("_default") on the embedded
	// server. Module-issued creds are not needed here; broad permissions
	// match historical behavior. See agent/internal/localauth.
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("nats dial: %v", err)
	}
	t.Cleanup(nc.Close)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			if err := nc.Publish("waypoint.rover-01.telemetry.drive", []byte("hello")); err != nil {
				return
			}
			_ = nc.Flush()
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()

	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var env envelope
	if err := c.ReadJSON(&env); err != nil {
		t.Fatalf("read: %v", err)
	}
	if env.Subject != "waypoint.rover-01.telemetry.drive" {
		t.Errorf("subject %q", env.Subject)
	}
	got, _ := base64.StdEncoding.DecodeString(env.Data)
	if string(got) != "hello" {
		t.Errorf("data %q", got)
	}
}

func TestGateway_unsubStopsDelivery(t *testing.T) {
	ns := newNATS(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", nil, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	wsURL := strings.Replace(httpsrv.URL, "http://", "ws://", 1) + "/ws?token=tok"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	must(t, c.WriteJSON(envelope{Action: "sub", Subject: "waypoint.rover-01.telemetry.drive"}))

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("nats dial: %v", err)
	}
	t.Cleanup(nc.Close)

	// Spin a reader goroutine that funnels arrivals into a channel.
	frames := make(chan envelope, 64)
	go func() {
		for {
			c.SetReadDeadline(time.Now().Add(2 * time.Second))
			var env envelope
			if err := c.ReadJSON(&env); err != nil {
				return
			}
			frames <- env
		}
	}()

	// Wait for the first delivery so we know the sub is live, retrying the
	// publish until the gateway's nc.Subscribe has actually registered.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("never received a frame")
		}
		_ = nc.Publish("waypoint.rover-01.telemetry.drive", []byte("pre-unsub"))
		_ = nc.Flush()
		select {
		case <-frames:
			goto subscribed
		case <-time.After(50 * time.Millisecond):
		}
	}
subscribed:
	must(t, c.WriteJSON(envelope{Action: "unsub", Subject: "waypoint.rover-01.telemetry.drive"}))

	// Drain any in-flight pre-unsub frames briefly, then assert silence under
	// a steady publish stream.
	drainUntil := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(drainUntil) {
		select {
		case <-frames:
		case <-time.After(50 * time.Millisecond):
		}
	}
	silentUntil := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(silentUntil) {
		_ = nc.Publish("waypoint.rover-01.telemetry.drive", []byte("post-unsub"))
		_ = nc.Flush()
		select {
		case got := <-frames:
			t.Fatalf("received frame after unsub: subject=%q", got.Subject)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestGateway_rejectsMissingToken(t *testing.T) {
	ns := newNATS(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", nil, nil, "", nil)
	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)
	wsURL := strings.Replace(httpsrv.URL, "http://", "ws://", 1) + "/ws"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail without token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		if resp != nil {
			t.Fatalf("status %d", resp.StatusCode)
		}
		t.Fatalf("no response")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newIssuer(t *testing.T) *session.LocalIssuer {
	t.Helper()
	iss, err := session.LoadOrCreate(filepath.Join(t.TempDir(), "local-account.nk"))
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	return iss
}

func TestGateway_NatsCred_ReturnsValidJWT(t *testing.T) {
	ns := newNATS(t)
	iss := newIssuer(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", iss, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	// Authorize via ?token= (mirrors /ws).
	req, _ := http.NewRequest("POST", httpsrv.URL+"/api/auth/nats-cred?token=tok", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"jwt", "seed", "expiresAt", "natsUrl"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("response missing %q: %v", key, body)
		}
	}

	tok, _ := body["jwt"].(string)
	seedB64, _ := body["seed"].(string)
	if tok == "" || seedB64 == "" {
		t.Fatalf("empty jwt or seed")
	}
	if _, err := base64.StdEncoding.DecodeString(seedB64); err != nil {
		t.Fatalf("seed not base64: %v", err)
	}

	claims, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatalf("decode jwt: %v", err)
	}
	if claims.IssuerAccount != iss.PublicKey() {
		t.Fatalf("issuer account = %q, want %q", claims.IssuerAccount, iss.PublicKey())
	}

	// Admin scope on rover-01.
	mustContain := func(list jwt.StringList, want string) {
		t.Helper()
		for _, s := range list {
			if s == want {
				return
			}
		}
		t.Errorf("missing %q in %v", want, list)
	}
	mustContain(claims.Permissions.Pub.Allow, "waypoint.rover-01.cmd.>")
	mustContain(claims.Permissions.Pub.Allow, "waypoint.rover-01.rpc.>")
	mustContain(claims.Permissions.Pub.Allow, "_INBOX.>")
	mustContain(claims.Permissions.Sub.Allow, "waypoint.rover-01.>")
	mustContain(claims.Permissions.Sub.Allow, "_INBOX.>")
}

func TestGateway_NatsCred_AcceptsCookie(t *testing.T) {
	ns := newNATS(t)
	iss := newIssuer(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", iss, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	sess, _ := gw.sessions.issue()
	req, _ := http.NewRequest("POST", httpsrv.URL+"/api/auth/nats-cred", nil)
	req.AddCookie(&http.Cookie{Name: "wp_local_token", Value: sess})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestGateway_NatsCred_Unauthorized(t *testing.T) {
	ns := newNATS(t)
	iss := newIssuer(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", iss, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	req, _ := http.NewRequest("POST", httpsrv.URL+"/api/auth/nats-cred", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestGateway_wildcardSubDeliversConcreteSubject(t *testing.T) {
	ns := newNATS(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", nil, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	wsURL := strings.Replace(httpsrv.URL, "http://", "ws://", 1) + "/ws?token=tok"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	// Subscribe to the rover-wide wildcard, like the dashboard Bus does.
	must(t, c.WriteJSON(envelope{Action: "sub", Subject: "waypoint.rover-01.>"}))

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("nats dial: %v", err)
	}
	t.Cleanup(nc.Close)

	// Republish on a tick to beat the async subscription registration.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			_ = nc.Publish("waypoint.rover-01.telemetry.power", []byte("x"))
			_ = nc.Flush()
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()

	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var env envelope
	if err := c.ReadJSON(&env); err != nil {
		t.Fatalf("read: %v", err)
	}
	if env.Subject != "waypoint.rover-01.telemetry.power" {
		t.Errorf("subject = %q, want concrete waypoint.rover-01.telemetry.power", env.Subject)
	}
}

func TestGateway_NatsCred_NoIssuerReturns503(t *testing.T) {
	ns := newNATS(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", nil, nil, "", nil)

	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	req, _ := http.NewRequest("POST", httpsrv.URL+"/api/auth/nats-cred?token=tok", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestGateway_WS_AcceptsCookie(t *testing.T) {
	ns := newNATS(t)
	gw := New(ns.ClientURL(), "rover-01", "tok", "", nil, nil, "", nil)
	httpsrv := httptest.NewServer(gw.HTTPHandler())
	t.Cleanup(httpsrv.Close)

	// No ?token= in the URL; auth comes from a valid session cookie alone.
	sess, _ := gw.sessions.issue()
	wsURL := strings.Replace(httpsrv.URL, "http://", "ws://", 1) + "/ws"
	hdr := http.Header{"Cookie": []string{"wp_local_token=" + sess}}
	c, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial with cookie failed: %v", err)
	}
	t.Cleanup(func() { c.Close() })
}

func TestGateway_WithLocalSession_SetsCookieOnDocument(t *testing.T) {
	gw := New("nats://unused", "rover-01", "tok", "", nil, nil, "", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(gw.WithLocalSession(next))
	t.Cleanup(srv.Close)

	// Document load (browser navigation) → cookie issued.
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got string
	for _, c := range resp.Cookies() {
		if c.Name == "wp_local_token" {
			got = c.Value
		}
	}
	// The cookie is an opaque session token, never the bootstrap token itself.
	if got == "tok" {
		t.Fatal("cookie must not be the literal bootstrap token")
	}
	if !gw.sessions.valid(got) {
		t.Fatalf("issued cookie %q is not a valid session", got)
	}

	// Asset request (no text/html) → no cookie spray.
	req2, _ := http.NewRequest("GET", srv.URL+"/assets/app.js", nil)
	req2.Header.Set("Accept", "*/*")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	for _, c := range resp2.Cookies() {
		if c.Name == "wp_local_token" {
			t.Fatal("cookie should not be set on asset requests")
		}
	}
}

func TestGateway_WithLocalSession_NoCookieWhenTokenEmpty(t *testing.T) {
	gw := New("nats://unused", "rover-01", "", "", nil, nil, "", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(gw.WithLocalSession(next))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "wp_local_token" {
			t.Fatal("no cookie expected when bootstrap token is empty")
		}
	}
}

func TestGateway_WithLocalSession_RejectsForeignHost(t *testing.T) {
	gw := New("nats://unused", "rover-01", "tok", "", nil, nil, "", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(gw.WithLocalSession(next))
	t.Cleanup(srv.Close)

	hasCookie := func(host string) bool {
		req, _ := http.NewRequest("GET", srv.URL+"/", nil)
		req.Host = host // simulate a DNS-rebinding / wrong-Host request
		req.Header.Set("Accept", "text/html")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		for _, c := range resp.Cookies() {
			if c.Name == "wp_local_token" {
				return true
			}
		}
		return false
	}

	if hasCookie("evil.com") {
		t.Fatal("cookie must NOT be issued to a foreign (public) Host")
	}
	if !hasCookie("waypoint.local") {
		t.Fatal("cookie should be issued for waypoint.local")
	}
	if !hasCookie("192.168.1.50") {
		t.Fatal("cookie should be issued for a private-IP Host")
	}
}

func TestGateway_CheckOrigin(t *testing.T) {
	gw := New("nats://unused", "rover-01", "tok", "", nil, nil, "", nil)
	check := gw.upgrader.CheckOrigin
	cases := map[string]bool{
		"":                         true,  // non-browser dial (no Origin)
		"http://waypoint.local":    true,  // mDNS name
		"http://192.168.1.50:5173": true,  // private LAN
		"http://100.100.0.1":       true,  // Tailscale CGNAT
		"http://localhost:5173":    true,  // dev
		"http://evil.com":          false, // foreign origin (rebinding)
		"https://attacker.example": false,
	}
	for origin, want := range cases {
		r, _ := http.NewRequest("GET", "/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if got := check(r); got != want {
			t.Errorf("CheckOrigin(Origin=%q) = %v, want %v", origin, got, want)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	cases := map[string]bool{
		"localhost":         true,
		"localhost:8080":    true,
		"waypoint.local":    true,
		"WAYPOINT.LOCAL":    true, // case-insensitive
		"waypoint.local.":   true, // FQDN trailing dot
		"rover-01.local:80": true,
		"evil.com.local":    false, // multi-label .local must not pass
		".local":            false, // empty label
		"localhost.":        true,
		"127.0.0.1":         true,
		"10.0.0.5":          true,
		"172.16.3.4":        true,
		"192.168.1.50:5173": true,
		"100.64.0.1":        true, // Tailscale CGNAT
		"100.127.255.255":   true,
		"[::1]:80":          true,
		"evil.com":          false,
		"evil.com:80":       false,
		"8.8.8.8":           false,
		"100.128.0.1":       false, // just past CGNAT
		"":                  false,
	}
	for in, want := range cases {
		if got := isLocalHost(in); got != want {
			t.Errorf("isLocalHost(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsLocalAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:5000": true,
		"192.168.1.5:80": true,
		"10.1.2.3:1":     true,
		"100.70.0.1:443": true, // Tailscale CGNAT
		"[::1]:80":       true,
		"8.8.8.8:80":     false,
		"203.0.113.5:9":  false,
		"100.128.0.1:1":  false, // just past CGNAT
		"not-an-addr":    false,
		"":               false,
	}
	for in, want := range cases {
		if got := isLocalAddr(in); got != want {
			t.Errorf("isLocalAddr(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestGateway_localAuthorized_GatesOnPeerIP(t *testing.T) {
	gw := New("nats://unused", "rover-01", "tok", "", nil, nil, "", nil)

	// A correct ?token= from a foreign peer IP is rejected.
	foreign, _ := http.NewRequest("GET", "/ws?token=tok", nil)
	foreign.RemoteAddr = "8.8.8.8:4444"
	if gw.localAuthorized(foreign) {
		t.Fatal("must reject a correct token from a non-local peer IP")
	}

	// Same token from a local peer IP is accepted.
	local, _ := http.NewRequest("GET", "/ws?token=tok", nil)
	local.RemoteAddr = "192.168.1.20:4444"
	if !gw.localAuthorized(local) {
		t.Fatal("must accept a correct token from a local peer IP")
	}

	// A valid session cookie from a local peer IP is accepted; the literal
	// bootstrap token in a cookie is NOT.
	sess, _ := gw.sessions.issue()
	cookieReq, _ := http.NewRequest("GET", "/ws", nil)
	cookieReq.RemoteAddr = "192.168.1.20:4444"
	cookieReq.AddCookie(&http.Cookie{Name: "wp_local_token", Value: sess})
	if !gw.localAuthorized(cookieReq) {
		t.Fatal("must accept a valid session cookie from a local peer")
	}
	bogus, _ := http.NewRequest("GET", "/ws", nil)
	bogus.RemoteAddr = "192.168.1.20:4444"
	bogus.AddCookie(&http.Cookie{Name: "wp_local_token", Value: "tok"})
	if gw.localAuthorized(bogus) {
		t.Fatal("the literal bootstrap token in a cookie must NOT authorize")
	}
}

func TestGateway_WithLocalSession_NoReissueWhenSessionValid(t *testing.T) {
	gw := New("nats://unused", "rover-01", "tok", "", nil, nil, "", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := gw.WithLocalSession(next)

	sess, _ := gw.sessions.issue()
	req := httptest.NewRequest("GET", "http://waypoint.local/", nil)
	req.RemoteAddr = "192.168.1.20:5000"
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: "wp_local_token", Value: sess})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("must not re-issue a cookie when a valid session is present")
	}
}
