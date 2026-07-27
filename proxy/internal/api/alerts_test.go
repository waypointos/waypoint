package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// fakeAlertsRepo is an in-memory stand-in for db.AlertsRepo. It implements the
// alertsRepo interface and records mutations so tests can assert side effects.
type fakeAlertsRepo struct {
	mu        sync.Mutex
	rows      map[string]db.AlertRow
	listErr   error
	getErr    error
	ackErr    error
	resErr    error
	lastAckID string
	lastAckBy uuid.UUID
	lastAckAt time.Time
	lastResID string
	lastResBy *uuid.UUID
	lastResAt time.Time
	lastResAR bool
}

func newFakeAlertsRepo(rows ...db.AlertRow) *fakeAlertsRepo {
	m := make(map[string]db.AlertRow, len(rows))
	for _, r := range rows {
		m[r.ID] = r
	}
	return &fakeAlertsRepo{rows: m}
}

func (f *fakeAlertsRepo) ListActive(_ context.Context, roverID string) ([]db.AlertRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]db.AlertRow, 0, len(f.rows))
	for _, r := range f.rows {
		if r.State == "resolved" {
			continue
		}
		if roverID != "" && r.RoverID != roverID {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeAlertsRepo) GetByID(_ context.Context, id string) (db.AlertRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return db.AlertRow{}, f.getErr
	}
	r, ok := f.rows[id]
	if !ok {
		return db.AlertRow{}, pgx.ErrNoRows
	}
	return r, nil
}

func (f *fakeAlertsRepo) Acknowledge(_ context.Context, id string, userID uuid.UUID, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ackErr != nil {
		return f.ackErr
	}
	r, ok := f.rows[id]
	if !ok {
		return pgx.ErrNoRows
	}
	r.State = "acknowledged"
	r.AcknowledgedAt = &at
	uid := userID
	r.AcknowledgedBy = &uid
	f.rows[id] = r
	f.lastAckID = id
	f.lastAckBy = userID
	f.lastAckAt = at
	return nil
}

func (f *fakeAlertsRepo) Resolve(_ context.Context, id string, userID *uuid.UUID, at time.Time, autoResolved bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resErr != nil {
		return f.resErr
	}
	r, ok := f.rows[id]
	if !ok {
		return pgx.ErrNoRows
	}
	r.State = "resolved"
	r.ResolvedAt = &at
	r.ResolvedBy = userID
	r.AutoResolved = autoResolved
	f.rows[id] = r
	f.lastResID = id
	f.lastResBy = userID
	f.lastResAt = at
	f.lastResAR = autoResolved
	return nil
}

// fakeAlertsAccess implements alertsAccessChecker over an in-memory grants map.
type fakeAlertsAccess struct {
	grants map[string]map[string]string // userID.String() -> roverID -> role
	err    error
}

func (a *fakeAlertsAccess) RoleFor(_ context.Context, userID uuid.UUID, roverID string) (string, error) {
	if a.err != nil {
		return "", a.err
	}
	g, ok := a.grants[userID.String()]
	if !ok {
		return "", nil
	}
	return g[roverID], nil
}

// captureNATS subscribes to a wildcard subject and returns a channel that
// receives each inbound message. The cleanup unsubscribes when the test ends.
func captureNATS(t *testing.T, nc *nats.Conn, subject string) <-chan *nats.Msg {
	t.Helper()
	ch := make(chan *nats.Msg, 4)
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case ch <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("subscribe %q: %v", subject, err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return ch
}

func mkAlertRow_T14(id, rover, sev, state string, raisedAt time.Time) db.AlertRow {
	return db.AlertRow{
		ID:       id,
		RoverID:  rover,
		Severity: sev,
		Source:   "agent.proxy",
		Code:     "proxy_disconnected",
		Message:  "test",
		Metadata: json.RawMessage(`{"hint":"x"}`),
		RaisedAt: raisedAt,
		State:    state,
	}
}

func newAlertsReq(t *testing.T, method, target string, user *authkit.CurrentUser, pathVars map[string]string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, v := range pathVars {
		req.SetPathValue(k, v)
	}
	if user != nil {
		req = req.WithContext(authkit.WithUser(context.Background(), user))
	}
	return req
}

func decodeAlertsBody(t *testing.T, rec *httptest.ResponseRecorder) []alertWireRow {
	t.Helper()
	var out []alertWireRow
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	return out
}

// --- list -----------------------------------------------------------------

func TestAlertsList_Admin_NoRover_ListsAll(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(
		mkAlertRow_T14("a", "sim-01", "critical", "active", now),
		mkAlertRow_T14("b", "sim-02", "warning", "active", now),
	)
	h := newAlertsListHandler(repo, &fakeAlertsAccess{})

	req := newAlertsReq(t, "GET", "/api/alerts", &authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAlertsBody(t, rec)
	if len(out) != 2 {
		t.Fatalf("got %d rows, want 2", len(out))
	}
}

func TestAlertsList_Admin_ScopesByRover(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(
		mkAlertRow_T14("a", "sim-01", "critical", "active", now),
		mkAlertRow_T14("b", "sim-02", "warning", "active", now),
	)
	h := newAlertsListHandler(repo, &fakeAlertsAccess{})

	req := newAlertsReq(t, "GET", "/api/alerts?rover=sim-01",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAlertsBody(t, rec)
	if len(out) != 1 || out[0].RoverID != "sim-01" {
		t.Fatalf("got %+v", out)
	}
}

func TestAlertsList_NonAdmin_WithAccess_OK(t *testing.T) {
	bob := uuid.New()
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(
		mkAlertRow_T14("a", "sim-01", "critical", "active", now),
	)
	access := &fakeAlertsAccess{grants: map[string]map[string]string{
		bob.String(): {"sim-01": "monitor"},
	}}
	h := newAlertsListHandler(repo, access)

	req := newAlertsReq(t, "GET", "/api/alerts?rover=sim-01",
		&authkit.CurrentUser{ID: bob, IsAdmin: false}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	out := decodeAlertsBody(t, rec)
	if len(out) != 1 || out[0].ID != "a" {
		t.Fatalf("got %+v", out)
	}
}

func TestAlertsList_NonAdmin_NoRover_BadRequest(t *testing.T) {
	h := newAlertsListHandler(newFakeAlertsRepo(), &fakeAlertsAccess{})
	req := newAlertsReq(t, "GET", "/api/alerts",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: false}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAlertsList_NonAdmin_NoAccess_Forbidden(t *testing.T) {
	bob := uuid.New()
	h := newAlertsListHandler(newFakeAlertsRepo(), &fakeAlertsAccess{
		grants: map[string]map[string]string{},
	})
	req := newAlertsReq(t, "GET", "/api/alerts?rover=sim-01",
		&authkit.CurrentUser{ID: bob, IsAdmin: false}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAlertsList_NoSession_Unauthorized(t *testing.T) {
	h := newAlertsListHandler(newFakeAlertsRepo(), &fakeAlertsAccess{})
	req := httptest.NewRequest("GET", "/api/alerts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAlertsList_BadLimit(t *testing.T) {
	h := newAlertsListHandler(newFakeAlertsRepo(), &fakeAlertsAccess{})
	req := newAlertsReq(t, "GET", "/api/alerts?limit=-5",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAlertsList_AccessLookupError_500(t *testing.T) {
	access := &fakeAlertsAccess{err: errors.New("db down")}
	h := newAlertsListHandler(newFakeAlertsRepo(), access)
	req := newAlertsReq(t, "GET", "/api/alerts?rover=sim-01",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: false}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAlertsList_RepoError_500(t *testing.T) {
	repo := newFakeAlertsRepo()
	repo.listErr = errors.New("boom")
	h := newAlertsListHandler(repo, &fakeAlertsAccess{})
	req := newAlertsReq(t, "GET", "/api/alerts",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAlertsList_WireShape(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ackAt := now.Add(time.Minute)
	uid := uuid.New()
	row := mkAlertRow_T14("a", "sim-01", "critical", "acknowledged", now)
	row.AcknowledgedAt = &ackAt
	row.AcknowledgedBy = &uid
	repo := newFakeAlertsRepo(row)

	h := newAlertsListHandler(repo, &fakeAlertsAccess{})
	req := newAlertsReq(t, "GET", "/api/alerts?rover=sim-01",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := decodeAlertsBody(t, rec)
	if len(out) != 1 {
		t.Fatalf("got %d rows", len(out))
	}
	got := out[0]
	if got.State != "acknowledged" {
		t.Errorf("State = %q", got.State)
	}
	if got.AcknowledgedAt == "" {
		t.Errorf("AcknowledgedAt empty")
	}
	if got.AcknowledgedBy != uid.String() {
		t.Errorf("AcknowledgedBy = %q", got.AcknowledgedBy)
	}
	if got.RaisedAt == "" {
		t.Errorf("RaisedAt empty")
	}
	if _, err := time.Parse(time.RFC3339, got.RaisedAt); err != nil {
		t.Errorf("RaisedAt not RFC3339: %v", err)
	}
}

// --- acknowledge ----------------------------------------------------------

func TestAlertsAck_Control_204_AndPublishes(t *testing.T) {
	nc := startEmbeddedNATS(t)
	bob := uuid.New()
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))
	access := &fakeAlertsAccess{grants: map[string]map[string]string{
		bob.String(): {"sim-01": "control"},
	}}

	msgs := captureNATS(t, nc, "waypoint.sim-01.alert.acknowledged")

	h := newAlertsAckHandler(repo, access, nc)
	req := newAlertsReq(t, "POST", "/api/alerts/a/acknowledge",
		&authkit.CurrentUser{ID: bob, IsAdmin: false}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if repo.lastAckID != "a" || repo.lastAckBy != bob {
		t.Fatalf("repo ack args = id=%q by=%v", repo.lastAckID, repo.lastAckBy)
	}
	select {
	case m := <-msgs:
		var ev waypointv1.AlertAcknowledged
		if err := proto.Unmarshal(m.Data, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.Id != "a" {
			t.Errorf("ev.Id = %q", ev.Id)
		}
		if ev.UserId != bob.String() {
			t.Errorf("ev.UserId = %q, want %q", ev.UserId, bob.String())
		}
		if ev.T == nil {
			t.Errorf("ev.T is nil")
		}
	case <-time.After(time.Second):
		t.Fatal("no NATS message within 1s")
	}
}

func TestAlertsAck_Admin_204(t *testing.T) {
	nc := startEmbeddedNATS(t)
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))

	msgs := captureNATS(t, nc, "waypoint.sim-01.alert.acknowledged")

	h := newAlertsAckHandler(repo, &fakeAlertsAccess{}, nc)
	req := newAlertsReq(t, "POST", "/api/alerts/a/acknowledge",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	select {
	case <-msgs:
	case <-time.After(time.Second):
		t.Fatal("no NATS message")
	}
}

func TestAlertsAck_MonitorRole_Forbidden(t *testing.T) {
	bob := uuid.New()
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))
	access := &fakeAlertsAccess{grants: map[string]map[string]string{
		bob.String(): {"sim-01": "monitor"},
	}}
	h := newAlertsAckHandler(repo, access, &noopPublisher{})

	req := newAlertsReq(t, "POST", "/api/alerts/a/acknowledge",
		&authkit.CurrentUser{ID: bob, IsAdmin: false}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if repo.lastAckID != "" {
		t.Errorf("repo Acknowledge must not be called on forbidden")
	}
}

func TestAlertsAck_UnknownID_404(t *testing.T) {
	repo := newFakeAlertsRepo() // empty
	h := newAlertsAckHandler(repo, &fakeAlertsAccess{}, &noopPublisher{})

	req := newAlertsReq(t, "POST", "/api/alerts/missing/acknowledge",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, map[string]string{"id": "missing"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAlertsAck_RepoAckError_500(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))
	repo.ackErr = errors.New("boom")
	h := newAlertsAckHandler(repo, &fakeAlertsAccess{}, &noopPublisher{})

	req := newAlertsReq(t, "POST", "/api/alerts/a/acknowledge",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAlertsAck_NoSession_401(t *testing.T) {
	h := newAlertsAckHandler(newFakeAlertsRepo(), &fakeAlertsAccess{}, &noopPublisher{})
	req := httptest.NewRequest("POST", "/api/alerts/a/acknowledge", nil)
	req.SetPathValue("id", "a")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

// --- resolve --------------------------------------------------------------

func TestAlertsResolve_Control_204_AndPublishes(t *testing.T) {
	nc := startEmbeddedNATS(t)
	bob := uuid.New()
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))
	access := &fakeAlertsAccess{grants: map[string]map[string]string{
		bob.String(): {"sim-01": "control"},
	}}

	msgs := captureNATS(t, nc, "waypoint.sim-01.alert.resolved")

	h := newAlertsResolveHandler(repo, access, nc)
	req := newAlertsReq(t, "POST", "/api/alerts/a/resolve",
		&authkit.CurrentUser{ID: bob, IsAdmin: false}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if repo.lastResID != "a" {
		t.Fatalf("repo Resolve id = %q", repo.lastResID)
	}
	if repo.lastResBy == nil || *repo.lastResBy != bob {
		t.Fatalf("repo Resolve userID = %v, want %v", repo.lastResBy, bob)
	}
	if repo.lastResAR != false {
		t.Errorf("auto_resolved must be false from API")
	}
	select {
	case m := <-msgs:
		var ev waypointv1.AlertResolved
		if err := proto.Unmarshal(m.Data, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.Id != "a" {
			t.Errorf("ev.Id = %q", ev.Id)
		}
		if ev.AutoResolved {
			t.Errorf("AutoResolved must be false in published message")
		}
		if ev.Reason != "user:"+bob.String() {
			t.Errorf("ev.Reason = %q, want user:%s", ev.Reason, bob.String())
		}
		if ev.T == nil {
			t.Errorf("ev.T is nil")
		}
	case <-time.After(time.Second):
		t.Fatal("no NATS message within 1s")
	}
}

func TestAlertsResolve_Admin_204(t *testing.T) {
	nc := startEmbeddedNATS(t)
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))

	msgs := captureNATS(t, nc, "waypoint.sim-01.alert.resolved")

	h := newAlertsResolveHandler(repo, &fakeAlertsAccess{}, nc)
	req := newAlertsReq(t, "POST", "/api/alerts/a/resolve",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	select {
	case <-msgs:
	case <-time.After(time.Second):
		t.Fatal("no NATS message")
	}
}

func TestAlertsResolve_MonitorRole_Forbidden(t *testing.T) {
	bob := uuid.New()
	now := time.Now().UTC()
	repo := newFakeAlertsRepo(mkAlertRow_T14("a", "sim-01", "critical", "active", now))
	access := &fakeAlertsAccess{grants: map[string]map[string]string{
		bob.String(): {"sim-01": "monitor"},
	}}
	h := newAlertsResolveHandler(repo, access, &noopPublisher{})

	req := newAlertsReq(t, "POST", "/api/alerts/a/resolve",
		&authkit.CurrentUser{ID: bob, IsAdmin: false}, map[string]string{"id": "a"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if repo.lastResID != "" {
		t.Errorf("repo Resolve must not be called on forbidden")
	}
}

func TestAlertsResolve_UnknownID_404(t *testing.T) {
	repo := newFakeAlertsRepo()
	h := newAlertsResolveHandler(repo, &fakeAlertsAccess{}, &noopPublisher{})

	req := newAlertsReq(t, "POST", "/api/alerts/missing/resolve",
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}, map[string]string{"id": "missing"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAlertsResolve_NoSession_401(t *testing.T) {
	h := newAlertsResolveHandler(newFakeAlertsRepo(), &fakeAlertsAccess{}, &noopPublisher{})
	req := httptest.NewRequest("POST", "/api/alerts/a/resolve", nil)
	req.SetPathValue("id", "a")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

// noopPublisher implements alertsPublisher for tests that exercise auth /
// not-found branches without needing a real broker.
type noopPublisher struct{}

func (n *noopPublisher) Publish(_ string, _ []byte) error { return nil }
