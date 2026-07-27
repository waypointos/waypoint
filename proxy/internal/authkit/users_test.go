package authkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/waypointos/waypoint/proxy/internal/db"
)

// stubInviter records the last invitation request and returns a canned result.
type stubInviter struct {
	calls       int
	lastEmail   string
	lastInviter string
	result      InviteResult
	err         error
}

func (s *stubInviter) SendInvitation(_ context.Context, email, inviterUserID string) (InviteResult, error) {
	s.calls++
	s.lastEmail = email
	s.lastInviter = inviterUserID
	if s.err != nil {
		return InviteResult{}, s.err
	}
	return s.result, nil
}

func sessionSecret() []byte {
	return []byte("0123456789012345678901234567890123456789")
}

// adminMux mounts a handler behind RequireUser + RequireAdmin so the auth path
// is exercised end-to-end, like in production.
func adminMux(deps *Deps, method, path string, h http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(method+" "+path,
		RequireUser(deps.Session, deps.Users, RequireAdmin(http.HandlerFunc(h))))
	return mux
}

// adminRequest builds a session-cookied http.Request, ready to feed into an
// admin-protected handler.
func adminRequest(t *testing.T, deps *Deps, userID, method, path string, body []byte) *http.Request {
	t.Helper()
	cookieRec := httptest.NewRecorder()
	if err := deps.Session.Set(cookieRec, Session{UserID: userID}); err != nil {
		t.Fatalf("session set: %v", err)
	}
	var br *bytes.Reader
	if body != nil {
		br = bytes.NewReader(body)
	} else {
		br = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, br)
	for _, c := range cookieRec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestListUsers_AdminGetsEveryoneWithGrants(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	rovers := db.NewRoversRepo(pool)
	access := db.NewAccessRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.UpsertFromWorkOS(ctx, "wk_bob", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rovers.Create(ctx, "sim-01", "Sim 1", "PK1", "", "", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err := access.Grant(ctx, bob.ID, "sim-01", "control", admin.ID); err != nil {
		t.Fatal(err)
	}

	deps := &Deps{
		Users:   users,
		Access:  access,
		Rovers:  rovers,
		Session: NewSessionCodec(sessionSecret(), false),
	}
	h := adminMux(deps, "GET", "/api/users", deps.ListUsers)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, adminRequest(t, deps, admin.ID.String(), "GET", "/api/users", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}

	var out []userListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(out) != 2 {
		t.Fatalf("want 2 users, got %d (%+v)", len(out), out)
	}
	// Ordered by email: admin@ then bob@.
	if out[0].Email != "admin@example.com" || out[1].Email != "bob@example.com" {
		t.Fatalf("unexpected order: %q,%q", out[0].Email, out[1].Email)
	}
	if !out[0].IsAdmin || out[1].IsAdmin {
		t.Fatalf("admin flag wrong: %+v", out)
	}
	if len(out[0].Access) != 0 {
		t.Fatalf("admin entry should have empty access slice (admin role is implicit), got %+v", out[0].Access)
	}
	if len(out[1].Access) != 1 || out[1].Access[0].RoverID != "sim-01" || out[1].Access[0].Role != "control" {
		t.Fatalf("bob access: %+v", out[1].Access)
	}
	if out[1].CreatedAt == "" {
		t.Fatal("createdAt should be populated")
	}
	if out[1].LastLoginAt == "" {
		t.Fatal("lastLoginAt should be populated after UpsertFromWorkOS")
	}
}

func TestListUsers_NonAdminGets403(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	access := db.NewAccessRepo(pool)

	if _, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com"); err != nil {
		t.Fatal(err)
	}
	bob, err := users.UpsertFromWorkOS(ctx, "wk_bob", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}

	deps := &Deps{
		Users:   users,
		Access:  access,
		Session: NewSessionCodec(sessionSecret(), false),
	}
	h := adminMux(deps, "GET", "/api/users", deps.ListUsers)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, adminRequest(t, deps, bob.ID.String(), "GET", "/api/users", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInviteUser_AdminSendsInvitation(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	access := db.NewAccessRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	stub := &stubInviter{result: InviteResult{
		AcceptURL: "https://workos.example/accept?token=abc",
		EmailSent: true,
	}}

	deps := &Deps{
		Users:           users,
		Access:          access,
		Session:         NewSessionCodec(sessionSecret(), false),
		Inviter: stub,
	}
	h := adminMux(deps, "POST", "/api/users/invite", deps.InviteUser)

	body, _ := json.Marshal(inviteRequest{Email: "  Newbie@Example.com "})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, adminRequest(t, deps, admin.ID.String(), "POST", "/api/users/invite", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}

	if stub.calls != 1 {
		t.Fatalf("inviter calls = %d, want 1", stub.calls)
	}
	if stub.lastEmail != "newbie@example.com" {
		t.Fatalf("inviter email = %q (want lower-cased trimmed)", stub.lastEmail)
	}
	if stub.lastInviter != "wk_admin" {
		t.Fatalf("inviter WorkOS id = %q, want wk_admin", stub.lastInviter)
	}

	var resp inviteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Email != "newbie@example.com" || !resp.EmailSent || resp.AcceptURL == "" {
		t.Fatalf("response = %+v", resp)
	}

	// Critical contract: invite must NOT pre-create a users row. The row is
	// created lazily on first sign-in via UpsertFromWorkOS.
	list, err := users.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range list {
		if u.Email == "newbie@example.com" {
			t.Fatalf("invite must not pre-create users row; found %+v", u)
		}
	}
}

func TestInviteUser_NonAdminGets403(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	access := db.NewAccessRepo(pool)

	if _, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com"); err != nil {
		t.Fatal(err)
	}
	bob, err := users.UpsertFromWorkOS(ctx, "wk_bob", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}

	stub := &stubInviter{result: InviteResult{EmailSent: true}}
	deps := &Deps{
		Users:           users,
		Access:          access,
		Session:         NewSessionCodec(sessionSecret(), false),
		Inviter: stub,
	}
	h := adminMux(deps, "POST", "/api/users/invite", deps.InviteUser)

	body, _ := json.Marshal(inviteRequest{Email: "x@y.com"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, adminRequest(t, deps, bob.ID.String(), "POST", "/api/users/invite", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	if stub.calls != 0 {
		t.Fatalf("non-admin must not reach the inviter; calls=%d", stub.calls)
	}
}

func TestInviteUser_BadInput(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	access := db.NewAccessRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	stub := &stubInviter{}
	deps := &Deps{
		Users:           users,
		Access:          access,
		Session:         NewSessionCodec(sessionSecret(), false),
		Inviter: stub,
	}
	h := adminMux(deps, "POST", "/api/users/invite", deps.InviteUser)

	t.Run("malformed json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, adminRequest(t, deps, admin.ID.String(), "POST", "/api/users/invite",
			[]byte("not json")))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("missing @", func(t *testing.T) {
		body, _ := json.Marshal(inviteRequest{Email: "no-at-sign"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, adminRequest(t, deps, admin.ID.String(), "POST", "/api/users/invite", body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "invalid email") {
			t.Fatalf("body = %q", rec.Body.String())
		}
	})

	t.Run("empty email", func(t *testing.T) {
		body, _ := json.Marshal(inviteRequest{Email: "   "})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, adminRequest(t, deps, admin.ID.String(), "POST", "/api/users/invite", body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status %d", rec.Code)
		}
	})

	if stub.calls != 0 {
		t.Fatalf("bad input must not reach the inviter; calls=%d", stub.calls)
	}
}

func TestInviteUser_InviterErrorSurfaces500(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	access := db.NewAccessRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	stub := &stubInviter{err: errors.New("workos down")}
	deps := &Deps{
		Users:           users,
		Access:          access,
		Session:         NewSessionCodec(sessionSecret(), false),
		Inviter: stub,
	}
	h := adminMux(deps, "POST", "/api/users/invite", deps.InviteUser)

	body, _ := json.Marshal(inviteRequest{Email: "x@y.com"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, adminRequest(t, deps, admin.ID.String(), "POST", "/api/users/invite", body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workos down") {
		t.Fatalf("body should surface upstream error; got %q", rec.Body.String())
	}
}
