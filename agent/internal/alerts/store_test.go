package alerts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func openMemStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mkAlert(id, rover string, at time.Time) StoredAlert {
	return StoredAlert{
		ID:       id,
		RoverID:  rover,
		Severity: SeverityCritical,
		Source:   "agent.proxy",
		Code:     "proxy_disconnected",
		Message:  "leaf disconnected",
		Metadata: json.RawMessage(`{"hint":"check uplink"}`),
		RaisedAt: at,
	}
}

func TestStore_RaiseThenListActive(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("a1", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := s.ListActive("sim-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	r := got[0]
	if r.ID != "a1" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.RoverID != "sim-01" {
		t.Errorf("RoverID = %q", r.RoverID)
	}
	if r.Severity != SeverityCritical {
		t.Errorf("Severity = %q", r.Severity)
	}
	if r.State != StateActive {
		t.Errorf("State = %q, want active", r.State)
	}
	if !r.RaisedAt.Equal(now) {
		t.Errorf("RaisedAt = %v, want %v", r.RaisedAt, now)
	}
	if string(r.Metadata) != `{"hint":"check uplink"}` {
		t.Errorf("Metadata = %s", r.Metadata)
	}
	if r.AcknowledgedAt != nil {
		t.Errorf("AcknowledgedAt should be nil, got %v", *r.AcknowledgedAt)
	}
}

func TestStore_RaiseSameID_UpdatesInPlace(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("dup", "sim-01", now)); err != nil {
		t.Fatalf("raise 1: %v", err)
	}

	updated := mkAlert("dup", "sim-01", now.Add(5*time.Second))
	updated.Severity = SeverityWarning
	updated.Message = "leaf reconnected? no — still down"
	updated.Metadata = json.RawMessage(`{"second":true}`)
	if err := s.Raise(updated); err != nil {
		t.Fatalf("raise 2: %v", err)
	}

	got, err := s.ListActive("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	r := got[0]
	if r.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want updated to warning", r.Severity)
	}
	if r.Message != "leaf reconnected? no — still down" {
		t.Errorf("Message = %q", r.Message)
	}
	if string(r.Metadata) != `{"second":true}` {
		t.Errorf("Metadata = %s", r.Metadata)
	}
	if !r.RaisedAt.Equal(now.Add(5 * time.Second)) {
		t.Errorf("RaisedAt = %v, want updated", r.RaisedAt)
	}
}

func TestStore_RaiseAfterResolve_Reopens(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("reopen", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := s.Resolve("reopen", "", now.Add(time.Minute), true); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Sanity check: it's gone from active list.
	active, err := s.ListActive("")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active after resolve, got %d", len(active))
	}

	// Re-raising the same id must reopen.
	if err := s.Raise(mkAlert("reopen", "sim-01", now.Add(2*time.Minute))); err != nil {
		t.Fatalf("re-raise: %v", err)
	}

	active, err = s.ListActive("")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active after re-raise, got %d", len(active))
	}
	r := active[0]
	if r.State != StateActive {
		t.Errorf("State = %q, want active", r.State)
	}
	if r.ResolvedAt != nil {
		t.Errorf("ResolvedAt should have been cleared, got %v", *r.ResolvedAt)
	}
	if r.AutoResolved {
		t.Errorf("AutoResolved should have been cleared")
	}
}

func TestStore_Acknowledge_Transitions(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("ack-me", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	ackTime := now.Add(30 * time.Second)
	if err := s.Acknowledge("ack-me", "user-uuid", ackTime); err != nil {
		t.Fatalf("ack: %v", err)
	}

	got, err := s.ListActive("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1 (acknowledged is still 'active' for the panel)", len(got))
	}
	r := got[0]
	if r.State != StateAcknowledged {
		t.Errorf("State = %q, want acknowledged", r.State)
	}
	if r.AcknowledgedAt == nil || !r.AcknowledgedAt.Equal(ackTime) {
		t.Errorf("AcknowledgedAt = %v, want %v", r.AcknowledgedAt, ackTime)
	}
	if r.AcknowledgedBy != "user-uuid" {
		t.Errorf("AcknowledgedBy = %q", r.AcknowledgedBy)
	}
}

func TestStore_Resolve_Transitions(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("res-me", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	resTime := now.Add(time.Minute)
	if err := s.Resolve("res-me", "", resTime, true); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Not in active list anymore.
	active, err := s.ListActive("")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active = %d, want 0 after resolve", len(active))
	}

	// But visible via history.
	hist, err := s.GetHistory("", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}
	r := hist[0]
	if r.State != StateResolved {
		t.Errorf("State = %q, want resolved", r.State)
	}
	if !r.AutoResolved {
		t.Errorf("AutoResolved = false, want true")
	}
	if r.ResolvedBy != "" {
		t.Errorf("ResolvedBy = %q, want empty for auto-resolve", r.ResolvedBy)
	}
	if r.ResolvedAt == nil || !r.ResolvedAt.Equal(resTime) {
		t.Errorf("ResolvedAt = %v, want %v", r.ResolvedAt, resTime)
	}
}

func TestStore_Resolve_ByUser_PreservesUserID(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.Raise(mkAlert("by-user", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := s.Resolve("by-user", "user-uuid", now.Add(time.Minute), false); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	hist, _ := s.GetHistory("sim-01", now.Add(-time.Hour))
	if len(hist) != 1 {
		t.Fatalf("history rows = %d", len(hist))
	}
	if hist[0].ResolvedBy != "user-uuid" {
		t.Errorf("ResolvedBy = %q", hist[0].ResolvedBy)
	}
	if hist[0].AutoResolved {
		t.Errorf("AutoResolved = true, want false")
	}
}

func TestStore_ListActive_FiltersByRover(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("a", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := s.Raise(mkAlert("b", "sim-01", now.Add(time.Second))); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := s.Raise(mkAlert("c", "sim-02", now.Add(2*time.Second))); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := s.ListActive("sim-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(sim-01) = %d, want 2", len(got))
	}
	for _, r := range got {
		if r.RoverID != "sim-01" {
			t.Errorf("got rover %q in filtered list", r.RoverID)
		}
	}

	all, err := s.ListActive("")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}
	// Newest first.
	if all[0].ID != "c" {
		t.Errorf("first id = %q, want c (newest)", all[0].ID)
	}
}

func TestStore_GetHistory_FiltersBySince(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Raise(mkAlert("old", "sim-01", now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := s.Raise(mkAlert("mid", "sim-01", now.Add(-30*time.Minute))); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := s.Raise(mkAlert("new", "sim-01", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := s.GetHistory("sim-01", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2 (mid + new)", len(got))
	}
	if got[0].ID != "new" {
		t.Errorf("first = %q, want new", got[0].ID)
	}
}

func TestStore_Acknowledge_UnknownIDIsNotFound(t *testing.T) {
	s := openMemStore(t)
	err := s.Acknowledge("nope", "user", time.Now())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestStore_Resolve_UnknownIDIsNotFound(t *testing.T) {
	s := openMemStore(t)
	err := s.Resolve("nope", "", time.Now(), true)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestSeverityToString(t *testing.T) {
	cases := []struct {
		in   int32
		want string
	}{
		{0, SeverityInfo},     // UNSPECIFIED → info (defensive default)
		{1, SeverityInfo},     // SEVERITY_INFO
		{2, SeverityWarning},  // SEVERITY_WARN
		{3, SeverityCritical}, // SEVERITY_FAULT
		{99, SeverityInfo},    // future/unknown value
	}
	for _, c := range cases {
		if got := SeverityToString(c.in); got != c.want {
			t.Errorf("SeverityToString(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStore_Raise_NoMetadataStoresNull(t *testing.T) {
	s := openMemStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	a := mkAlert("nometa", "sim-01", now)
	a.Metadata = nil
	if err := s.Raise(a); err != nil {
		t.Fatalf("raise: %v", err)
	}
	got, err := s.ListActive("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	if got[0].Metadata != nil {
		t.Errorf("Metadata = %s, want nil", got[0].Metadata)
	}
}
