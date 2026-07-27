package router_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/landing"
	"github.com/waypointos/waypoint/proxy/internal/router"
)

// stub Authenticator for /auth/login + /auth/callback.
type stubAuth struct {
	authURL string
	result  *authkit.AuthResult
}

func (s *stubAuth) AuthorizationURL(state string) string { return s.authURL + "?state=" + state }
func (s *stubAuth) Authenticate(ctx context.Context, code string) (*authkit.AuthResult, error) {
	return s.result, nil
}

type fixture struct {
	handler http.Handler
	session *authkit.SessionCodec
	users   *db.UsersRepo
	pool    *pgxpool.Pool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	c, err := pgcontainer.RunContainer(ctx,
		pgcontainer.WithDatabase("waypoint"),
		pgcontainer.WithUsername("waypoint"),
		pgcontainer.WithPassword("dev"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	dsn, _ := c.ConnectionString(ctx, "sslmode=disable")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := pool.Ping(ctx); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	users := db.NewUsersRepo(pool)
	secret := []byte("0123456789012345678901234567890123456789")
	session := authkit.NewSessionCodec(secret, false)
	stateCodec := authkit.NewStateCodec(secret, false)
	deps := &authkit.Deps{
		WorkOS:       &stubAuth{authURL: "https://api.workos.com/sso/authorize", result: &authkit.AuthResult{WorkOSUserID: "wk_alice", Email: "alice@example.com"}},
		Users:        users,
		Session:      session,
		StateCodec:   stateCodec,
		CookieSecure: false,
	}

	landingH := landing.New()
	rt := router.New(session, users)
	// Simulated SPA handler returning a tiny index.html.
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Content-Type", "application/javascript")
			w.Write([]byte("// spa asset"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><div id="root"></div>`))
	})

	rt.Public("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	rt.Public("GET /landing/static/", http.StripPrefix("/landing/static/", landingH.Static()))
	rt.Public("GET /auth/login", http.HandlerFunc(deps.Login))
	rt.Public("GET /auth/callback", http.HandlerFunc(deps.Callback))
	rt.RequireUser("GET /api/me", http.HandlerFunc(deps.Me))
	rt.RequireUserHTML("GET /assets/", spa)
	rt.SPAGate(spa, http.HandlerFunc(landingH.Index))

	return &fixture{
		handler: router.TrustForwarded(rt.Handler()),
		session: session,
		users:   users,
		pool:    pool,
	}
}

func (f *fixture) seedUser(t *testing.T) *db.User {
	t.Helper()
	u, err := f.users.UpsertFromWorkOS(context.Background(), "wk_alice", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (f *fixture) sessionCookie(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := f.session.Set(rec, authkit.Session{UserID: userID}); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "wp_session" {
			return c
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func TestPolicy(t *testing.T) {
	f := newFixture(t)
	u := f.seedUser(t)
	sessCookie := f.sessionCookie(t, u.ID.String())

	cases := []struct {
		name        string
		method      string
		path        string
		withSession bool
		wantStatus  int
		bodyMust    string
		locPrefix   string
	}{
		{"root anon → landing", "GET", "/", false, 200, "WAYPOINT", ""},
		{"root anon body includes login anchor", "GET", "/", false, 200, `href="/auth/login"`, ""},
		{"root authed → SPA", "GET", "/", true, 200, `<div id="root">`, ""},
		{"deep link anon → 302", "GET", "/rover/abc", false, 302, "", "/auth/login?next="},
		{"deep link authed → SPA", "GET", "/rover/abc", true, 200, `<div id="root">`, ""},
		{"unknown deep link anon → 302", "GET", "/unknown/path", false, 302, "", "/auth/login?next="},
		{"unknown deep link authed → SPA", "GET", "/unknown/path", true, 200, `<div id="root">`, ""},
		{"admin route anon → 302", "GET", "/admin", false, 302, "", "/auth/login?next="},
		{"admin route authed → SPA", "GET", "/admin", true, 200, `<div id="root">`, ""},
		{"asset anon → 302", "GET", "/assets/x.js", false, 302, "", "/auth/login"},
		{"asset authed → 200", "GET", "/assets/x.js", true, 200, "// spa asset", ""},
		{"healthz", "GET", "/healthz", false, 200, "ok", ""},
		{"login starts WorkOS", "GET", "/auth/login", false, 302, "", "https://api.workos.com/"},
		{"api/me anon → 401", "GET", "/api/me", false, 401, "", ""},
		{"api/me authed → 200", "GET", "/api/me", true, 200, "alice@example.com", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			if c.withSession {
				req.AddCookie(sessCookie)
			}
			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, req)

			if rec.Code != c.wantStatus {
				body, _ := io.ReadAll(rec.Result().Body)
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, c.wantStatus, body)
			}
			if c.bodyMust != "" && !strings.Contains(rec.Body.String(), c.bodyMust) {
				t.Errorf("body missing %q\ngot: %s", c.bodyMust, rec.Body.String())
			}
			if c.locPrefix != "" {
				if got := rec.Header().Get("Location"); !strings.HasPrefix(got, c.locPrefix) {
					t.Errorf("Location = %q, want prefix %q", got, c.locPrefix)
				}
			}
		})
	}
}

func TestCallbackHonorsNextFromStateCookie(t *testing.T) {
	f := newFixture(t)

	loginReq := httptest.NewRequest("GET", "/auth/login?next=/rover/abc", nil)
	loginRec := httptest.NewRecorder()
	f.handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != 302 {
		t.Fatalf("login status = %d", loginRec.Code)
	}
	var stateCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "wp_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatal("wp_state missing")
	}

	loc := loginRec.Header().Get("Location")
	idx := strings.Index(loc, "state=")
	if idx < 0 {
		t.Fatalf("no state in Location: %q", loc)
	}
	stateValue := loc[idx+len("state="):]

	cbReq := httptest.NewRequest("GET", "/auth/callback?state="+stateValue+"&code=abc", nil)
	cbReq.AddCookie(stateCookie)
	cbRec := httptest.NewRecorder()
	f.handler.ServeHTTP(cbRec, cbReq)

	if cbRec.Code != 302 {
		t.Fatalf("callback status = %d, body = %s", cbRec.Code, cbRec.Body.String())
	}
	if loc := cbRec.Header().Get("Location"); loc != "/rover/abc" {
		t.Errorf("Location = %q, want /rover/abc", loc)
	}
}

func TestCallbackRejectsHostileNext(t *testing.T) {
	f := newFixture(t)

	loginRec := httptest.NewRecorder()
	f.handler.ServeHTTP(loginRec, httptest.NewRequest("GET", "/auth/login?next=//evil.com/x", nil))
	var stateCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "wp_state" {
			stateCookie = c
		}
	}
	loc := loginRec.Header().Get("Location")
	stateValue := loc[strings.Index(loc, "state=")+len("state="):]

	cbReq := httptest.NewRequest("GET", "/auth/callback?state="+stateValue+"&code=abc", nil)
	cbReq.AddCookie(stateCookie)
	cbRec := httptest.NewRecorder()
	f.handler.ServeHTTP(cbRec, cbReq)
	if got := cbRec.Header().Get("Location"); got != "/" {
		t.Errorf("hostile next was not sanitized; Location = %q", got)
	}
}
