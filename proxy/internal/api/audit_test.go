package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

// fakeAuditRepo is an in-memory stand-in for db.AuditRepo. It applies the
// same filters the real repo would: rover, since, limit, sorted by
// occurred_at DESC.
type fakeAuditRepo struct {
	rows   []db.AuditRow
	lastFn db.AuditFilter
}

func (f *fakeAuditRepo) List(_ context.Context, q db.AuditFilter) ([]db.AuditRow, error) {
	f.lastFn = q
	out := make([]db.AuditRow, 0, len(f.rows))
	for _, r := range f.rows {
		if q.RoverID != "" && r.RoverID != q.RoverID {
			continue
		}
		if q.SinceTime != nil && r.OccurredAt.Before(*q.SinceTime) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurredAt.After(out[j].OccurredAt) })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// fakeAuditAccess implements auditAccessChecker over an in-memory grant table.
type fakeAuditAccess struct {
	grants map[string]map[string]string // userID.String() -> roverID -> role
	err    error
}

func (a *fakeAuditAccess) RoleFor(_ context.Context, userID uuid.UUID, roverID string) (string, error) {
	if a.err != nil {
		return "", a.err
	}
	g, ok := a.grants[userID.String()]
	if !ok {
		return "", nil
	}
	return g[roverID], nil
}

func mkAuditRow(rover, kind, source string, when time.Time, actor *uuid.UUID, payload string) db.AuditRow {
	return db.AuditRow{
		ID:          uuid.New(),
		OccurredAt:  when,
		Source:      source,
		ActorUserID: actor,
		RoverID:     rover,
		Kind:        kind,
		Payload:     json.RawMessage(payload),
	}
}

func auditRequest(t *testing.T, target string, user *authkit.CurrentUser) *http.Request {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	if user != nil {
		req = req.WithContext(authkit.WithUser(context.Background(), user))
	}
	return req
}

func decodeAuditBody(t *testing.T, rec *httptest.ResponseRecorder) []auditWireRow {
	t.Helper()
	var out []auditWireRow
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	return out
}

func TestAudit_Admin_WithRover(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeAuditRepo{rows: []db.AuditRow{
		mkAuditRow("sim-01", "command", "proxy-ws", now, nil, `{"a":1}`),
		mkAuditRow("sim-02", "command", "proxy-ws", now, nil, `{"a":2}`),
	}}
	access := &fakeAuditAccess{}

	h := newAuditListHandler(repo, access)
	req := auditRequest(t, "/api/audit?rover=sim-01",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAuditBody(t, rec)
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1", len(out))
	}
	if out[0].RoverID != "sim-01" {
		t.Fatalf("RoverID = %q", out[0].RoverID)
	}
	if repo.lastFn.RoverID != "sim-01" {
		t.Fatalf("filter RoverID = %q", repo.lastFn.RoverID)
	}
	if repo.lastFn.Limit != 500 {
		t.Fatalf("default limit = %d", repo.lastFn.Limit)
	}
}

func TestAudit_Admin_NoRover(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeAuditRepo{rows: []db.AuditRow{
		mkAuditRow("sim-01", "command", "proxy-ws", now, nil, `{}`),
		mkAuditRow("sim-02", "command", "proxy-ws", now.Add(-time.Second), nil, `{}`),
	}}
	h := newAuditListHandler(repo, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAuditBody(t, rec)
	if len(out) != 2 {
		t.Fatalf("got %d rows, want 2", len(out))
	}
	// DESC by occurred_at
	if out[0].RoverID != "sim-01" {
		t.Fatalf("first row rover = %q, want sim-01 (most recent)", out[0].RoverID)
	}
}

func TestAudit_NonAdmin_NoAccess_Forbidden(t *testing.T) {
	carol := uuid.New()
	repo := &fakeAuditRepo{}
	access := &fakeAuditAccess{grants: map[string]map[string]string{}}

	h := newAuditListHandler(repo, access)
	req := auditRequest(t, "/api/audit?rover=sim-01",
		&authkit.CurrentUser{ID: carol, IsAdmin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAudit_NonAdmin_WithAccess_OK(t *testing.T) {
	bob := uuid.New()
	now := time.Now().UTC()
	repo := &fakeAuditRepo{rows: []db.AuditRow{
		mkAuditRow("sim-01", "command", "proxy-ws", now, &bob, `{}`),
		mkAuditRow("sim-02", "command", "proxy-ws", now, nil, `{}`),
	}}
	access := &fakeAuditAccess{grants: map[string]map[string]string{
		bob.String(): {"sim-01": "control"},
	}}

	h := newAuditListHandler(repo, access)
	req := auditRequest(t, "/api/audit?rover=sim-01",
		&authkit.CurrentUser{ID: bob, IsAdmin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAuditBody(t, rec)
	if len(out) != 1 || out[0].RoverID != "sim-01" {
		t.Fatalf("got %+v, want one sim-01 row", out)
	}
	if out[0].ActorUserID != bob.String() {
		t.Fatalf("ActorUserID = %q, want %q", out[0].ActorUserID, bob.String())
	}
}

func TestAudit_NonAdmin_NoRover_BadRequest(t *testing.T) {
	bob := uuid.New()
	h := newAuditListHandler(&fakeAuditRepo{}, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit",
		&authkit.CurrentUser{ID: bob, IsAdmin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAudit_SinceFilter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-time.Hour)
	recent := now.Add(-time.Minute)
	repo := &fakeAuditRepo{rows: []db.AuditRow{
		mkAuditRow("sim-01", "command", "proxy-ws", old, nil, `{}`),
		mkAuditRow("sim-01", "command", "proxy-ws", recent, nil, `{}`),
		mkAuditRow("sim-01", "command", "proxy-ws", now, nil, `{}`),
	}}
	cutoff := now.Add(-5 * time.Minute).Format(time.RFC3339)

	h := newAuditListHandler(repo, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit?rover=sim-01&since="+cutoff,
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAuditBody(t, rec)
	if len(out) != 2 {
		t.Fatalf("got %d rows, want 2 (recent + now)", len(out))
	}
	if repo.lastFn.SinceTime == nil {
		t.Fatal("SinceTime was not parsed into the filter")
	}
}

func TestAudit_LimitParam(t *testing.T) {
	now := time.Now().UTC()
	rows := make([]db.AuditRow, 5)
	for i := range rows {
		rows[i] = mkAuditRow("sim-01", "command", "proxy-ws",
			now.Add(time.Duration(-i)*time.Second), nil, `{}`)
	}
	repo := &fakeAuditRepo{rows: rows}

	h := newAuditListHandler(repo, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit?rover=sim-01&limit=2",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAuditBody(t, rec)
	if len(out) != 2 {
		t.Fatalf("got %d rows, want 2", len(out))
	}
	if repo.lastFn.Limit != 2 {
		t.Fatalf("filter limit = %d, want 2", repo.lastFn.Limit)
	}
}

func TestAudit_LimitCapsAt1000(t *testing.T) {
	repo := &fakeAuditRepo{}
	h := newAuditListHandler(repo, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit?rover=sim-01&limit=5000",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if repo.lastFn.Limit != 1000 {
		t.Fatalf("filter limit = %d, want 1000 (capped)", repo.lastFn.Limit)
	}
}

func TestAudit_BadSince(t *testing.T) {
	h := newAuditListHandler(&fakeAuditRepo{}, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit?rover=sim-01&since=not-a-date",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "RFC3339") {
		t.Fatalf("body = %q, want hint about RFC3339", rec.Body.String())
	}
}

func TestAudit_BadLimit(t *testing.T) {
	h := newAuditListHandler(&fakeAuditRepo{}, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit?rover=sim-01&limit=-3",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAudit_NoSession(t *testing.T) {
	h := newAuditListHandler(&fakeAuditRepo{}, &fakeAuditAccess{})
	req := httptest.NewRequest("GET", "/api/audit", nil) // no ctx user
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAudit_AccessLookupError(t *testing.T) {
	bob := uuid.New()
	access := &fakeAuditAccess{err: errors.New("db down")}
	h := newAuditListHandler(&fakeAuditRepo{}, access)
	req := auditRequest(t, "/api/audit?rover=sim-01",
		&authkit.CurrentUser{ID: bob, IsAdmin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAudit_PayloadIsRawJSON(t *testing.T) {
	repo := &fakeAuditRepo{rows: []db.AuditRow{
		mkAuditRow("sim-01", "command", "proxy-ws", time.Now().UTC(), nil,
			`{"subject":"waypoint.sim-01.cmd.drive","payload_bytes":7}`),
	}}
	h := newAuditListHandler(repo, &fakeAuditAccess{})
	req := auditRequest(t, "/api/audit?rover=sim-01",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := decodeAuditBody(t, rec)
	if len(out) != 1 {
		t.Fatalf("got %d rows", len(out))
	}
	var pm map[string]any
	if err := json.Unmarshal(out[0].Payload, &pm); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if pm["subject"] != "waypoint.sim-01.cmd.drive" {
		t.Fatalf("payload subject = %v", pm["subject"])
	}
}
