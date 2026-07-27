package authkit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/waypointos/waypoint/proxy/internal/db"
)

// stubAuth is a test double for the Authenticator interface, used by callback
// tests. Returning canned values lets us exercise handlers.go without making
// network calls to WorkOS.
type stubAuth struct {
	authzURL string
	result   *AuthResult
	err      error
	gotState string
	gotCode  string
}

func (s *stubAuth) AuthorizationURL(state string) string {
	s.gotState = state
	return s.authzURL
}

func (s *stubAuth) Authenticate(_ context.Context, code string) (*AuthResult, error) {
	s.gotCode = code
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

// Compile-time assertion that *WorkOS satisfies the new interface.
var _ Authenticator = (*WorkOS)(nil)
var _ Authenticator = (*stubAuth)(nil)

func TestLogin_RedirectsToWorkOSAndSetsStateCookie(t *testing.T) {
	auth := &stubAuth{authzURL: "https://api.workos.com/sso/authorize?x=1"}
	state := NewStateCodec([]byte("0123456789012345678901234567890123456789"), false)
	d := &Deps{
		WorkOS:       auth,
		StateCodec:   state,
		CookieSecure: false,
	}

	req := httptest.NewRequest("GET", "/auth/login?next=/rover/abc", nil)
	rec := httptest.NewRecorder()
	d.Login(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.HasPrefix(got, "https://api.workos.com/") {
		t.Fatalf("Location = %q", got)
	}

	// state cookie must decode and carry the next URL.
	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "wp_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil {
		t.Fatal("wp_state cookie missing")
	}
	decoded, err := state.Decode(stateCookie.Value)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Next != "/rover/abc" {
		t.Errorf("Next = %q, want /rover/abc", decoded.Next)
	}
	if decoded.State == "" {
		t.Error("State is empty")
	}
	if decoded.State != auth.gotState {
		t.Errorf("State sent to WorkOS (%q) doesn't match cookie (%q)", auth.gotState, decoded.State)
	}
}

func TestLogin_SanitizesHostileNext(t *testing.T) {
	auth := &stubAuth{authzURL: "https://x/"}
	state := NewStateCodec([]byte("0123456789012345678901234567890123456789"), false)
	d := &Deps{WorkOS: auth, StateCodec: state}

	req := httptest.NewRequest("GET", "/auth/login?next=//evil.com/x", nil)
	rec := httptest.NewRecorder()
	d.Login(rec, req)

	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "wp_state" {
			stateCookie = c
		}
	}
	decoded, _ := state.Decode(stateCookie.Value)
	if decoded.Next != "/" {
		t.Fatalf("Next = %q, want /", decoded.Next)
	}
}

func TestCallback_RedirectsToNext(t *testing.T) {
	auth := &stubAuth{result: &AuthResult{WorkOSUserID: "wk_alice", Email: "alice@example.com"}}
	state := NewStateCodec([]byte("0123456789012345678901234567890123456789"), false)
	pool := newTestPool(t)
	users := db.NewUsersRepo(pool)
	session := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)
	d := &Deps{
		WorkOS:     auth,
		Users:      users,
		Session:    session,
		StateCodec: state,
	}

	// Mint a state cookie with next=/rover/abc.
	rec := httptest.NewRecorder()
	if err := state.Set(rec, StatePayload{State: "xyz", Next: "/rover/abc"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/auth/callback?state=xyz&code=abc", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	out := httptest.NewRecorder()
	d.Callback(out, req)

	if out.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", out.Code)
	}
	if loc := out.Header().Get("Location"); loc != "/rover/abc" {
		t.Errorf("Location = %q, want /rover/abc", loc)
	}
	// wp_session cookie must be set.
	var sess *http.Cookie
	for _, c := range out.Result().Cookies() {
		if c.Name == "wp_session" {
			sess = c
		}
	}
	if sess == nil {
		t.Fatal("wp_session cookie missing")
	}
}

func TestCallback_RejectsStateMismatch(t *testing.T) {
	state := NewStateCodec([]byte("0123456789012345678901234567890123456789"), false)
	d := &Deps{StateCodec: state}

	rec := httptest.NewRecorder()
	_ = state.Set(rec, StatePayload{State: "expected", Next: "/"})

	req := httptest.NewRequest("GET", "/auth/callback?state=different&code=abc", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	out := httptest.NewRecorder()
	d.Callback(out, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", out.Code)
	}
}

func TestCallback_RejectsMissingStateCookie(t *testing.T) {
	state := NewStateCodec([]byte("0123456789012345678901234567890123456789"), false)
	d := &Deps{StateCodec: state}
	req := httptest.NewRequest("GET", "/auth/callback?state=xyz&code=abc", nil)
	out := httptest.NewRecorder()
	d.Callback(out, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", out.Code)
	}
}

func TestRequireUserHTML_RedirectsAnonymousWithNext(t *testing.T) {
	pool := newTestPool(t)
	users := db.NewUsersRepo(pool)
	session := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)

	called := false
	guarded := RequireUserHTML(session, users, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/rover/abc?tab=video", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	if called {
		t.Fatal("inner handler should not be called for anonymous request")
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	want := "/auth/login?next=" + url.QueryEscape("/rover/abc?tab=video")
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestRequireUserHTML_PassesThroughAuthenticated(t *testing.T) {
	pool := newTestPool(t)
	users := db.NewUsersRepo(pool)
	user, err := users.UpsertFromWorkOS(context.Background(), "wk_alice", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	session := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)

	called := false
	guarded := RequireUserHTML(session, users, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	if err := session.Set(rec, Session{UserID: user.ID.String()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/rover/abc", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	out := httptest.NewRecorder()
	guarded.ServeHTTP(out, req)

	if !called {
		t.Fatal("inner handler should be called for authenticated request")
	}
	if out.Code != http.StatusOK {
		t.Fatalf("status = %d", out.Code)
	}
}
