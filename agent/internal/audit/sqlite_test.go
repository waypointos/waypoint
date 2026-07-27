package audit

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func openMem(t *testing.T) *Store {
	t.Helper()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStore_InsertList_RoundTrip(t *testing.T) {
	s := openMem(t)

	now := time.Now().UTC().Truncate(time.Second)
	row := Row{
		ID:          "id-1",
		OccurredAt:  now,
		Source:      "agent-rpc",
		ActorUserID: "", // system-issued — should round-trip as empty
		ActorEmail:  "",
		RoverID:     "sim-01",
		Kind:        "command",
		Payload:     json.RawMessage(`{"subject":"waypoint.sim-01.cmd.drive","payload_bytes":42}`),
	}
	if err := s.Insert(row); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	r := got[0]
	if r.ID != row.ID {
		t.Errorf("ID = %q, want %q", r.ID, row.ID)
	}
	if !r.OccurredAt.Equal(now) {
		t.Errorf("OccurredAt = %v, want %v", r.OccurredAt, now)
	}
	if r.Source != row.Source {
		t.Errorf("Source = %q, want %q", r.Source, row.Source)
	}
	if r.ActorUserID != "" {
		t.Errorf("ActorUserID = %q, want empty (NULL round-trips to empty)", r.ActorUserID)
	}
	if r.ActorEmail != "" {
		t.Errorf("ActorEmail = %q, want empty", r.ActorEmail)
	}
	if r.RoverID != row.RoverID {
		t.Errorf("RoverID = %q, want %q", r.RoverID, row.RoverID)
	}
	if r.Kind != row.Kind {
		t.Errorf("Kind = %q, want %q", r.Kind, row.Kind)
	}
	if string(r.Payload) != string(row.Payload) {
		t.Errorf("Payload = %s, want %s", r.Payload, row.Payload)
	}
}

func TestStore_InsertList_KeepsActor(t *testing.T) {
	s := openMem(t)
	now := time.Now().UTC().Truncate(time.Second)
	row := Row{
		ID:          "id-actor",
		OccurredAt:  now,
		Source:      "proxy-ws",
		ActorUserID: "user-uuid",
		ActorEmail:  "alice@example.com",
		RoverID:     "sim-01",
		Kind:        "command",
		Payload:     json.RawMessage(`{}`),
	}
	if err := s.Insert(row); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	if got[0].ActorUserID != "user-uuid" {
		t.Errorf("ActorUserID = %q, want %q", got[0].ActorUserID, "user-uuid")
	}
	if got[0].ActorEmail != "alice@example.com" {
		t.Errorf("ActorEmail = %q, want %q", got[0].ActorEmail, "alice@example.com")
	}
}

func TestStore_ListFilters_Rover(t *testing.T) {
	s := openMem(t)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id, rover string) Row {
		return Row{
			ID: id, OccurredAt: now, Source: "agent-rpc",
			RoverID: rover, Kind: "command", Payload: json.RawMessage(`{}`),
		}
	}
	must := func(err error) {
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	must(s.Insert(mk("a1", "sim-01")))
	must(s.Insert(mk("a2", "sim-01")))
	must(s.Insert(mk("a3", "sim-01")))
	must(s.Insert(mk("b1", "sim-02")))

	got, err := s.List(Filter{RoverID: "sim-01"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for _, r := range got {
		if r.RoverID != "sim-01" {
			t.Errorf("rover = %q in filtered list", r.RoverID)
		}
	}

	all, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("len(all) = %d, want 4", len(all))
	}
}

func TestStore_ListFilters_Since(t *testing.T) {
	s := openMem(t)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id string, t time.Time) Row {
		return Row{
			ID: id, OccurredAt: t, Source: "agent-rpc",
			RoverID: "sim-01", Kind: "command", Payload: json.RawMessage(`{}`),
		}
	}
	if err := s.Insert(mk("old", now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := s.Insert(mk("midA", now.Add(-30*time.Minute))); err != nil {
		t.Fatalf("insert midA: %v", err)
	}
	if err := s.Insert(mk("midB", now.Add(-15*time.Minute))); err != nil {
		t.Fatalf("insert midB: %v", err)
	}
	if err := s.Insert(mk("new", now)); err != nil {
		t.Fatalf("insert new: %v", err)
	}

	cutoff := now.Add(-1 * time.Hour)
	got, err := s.List(Filter{SinceTime: &cutoff})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (midA, midB, new)", len(got))
	}
	for _, r := range got {
		if r.OccurredAt.Before(cutoff) {
			t.Errorf("row %q before cutoff %v", r.ID, cutoff)
		}
	}
	// Verify newest-first ordering.
	if got[0].ID != "new" || got[2].ID != "midA" {
		t.Errorf("order: got [%s, %s, %s], want [new, midB, midA]", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestStore_Insert_IdempotentOnDuplicateID(t *testing.T) {
	s := openMem(t)
	now := time.Now().UTC().Truncate(time.Second)
	row := Row{
		ID: "dup", OccurredAt: now, Source: "agent-rpc",
		RoverID: "sim-01", Kind: "command", Payload: json.RawMessage(`{"first":true}`),
	}
	if err := s.Insert(row); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Same ID, different payload — must be a no-op (ON CONFLICT DO NOTHING).
	row2 := row
	row2.Payload = json.RawMessage(`{"second":true}`)
	if err := s.Insert(row2); err != nil {
		t.Fatalf("second insert: %v", err)
	}
	got, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if string(got[0].Payload) != `{"first":true}` {
		t.Errorf("payload = %s, want first insert preserved", got[0].Payload)
	}
}

func TestStore_Sweep_RemovesOldRows(t *testing.T) {
	s := openMem(t)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id string, t time.Time) Row {
		return Row{
			ID: id, OccurredAt: t, Source: "agent-rpc",
			RoverID: "sim-01", Kind: "command", Payload: json.RawMessage(`{}`),
		}
	}
	must := func(err error) {
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	// 35d old should be swept; everything else stays.
	must(s.Insert(mk("d35", now.Add(-35*24*time.Hour))))
	must(s.Insert(mk("d25", now.Add(-25*24*time.Hour))))
	must(s.Insert(mk("d10", now.Add(-10*24*time.Hour))))
	must(s.Insert(mk("h1", now.Add(-1*time.Hour))))
	must(s.Insert(mk("now", now)))

	n, err := s.Sweep(30*24*time.Hour, 100)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Errorf("Sweep deleted %d rows, want 1", n)
	}
	got, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d after sweep, want 4", len(got))
	}
	for _, r := range got {
		if r.ID == "d35" {
			t.Errorf("d35 should have been swept")
		}
	}
}

func TestStore_Sweep_EnforcesMaxRows(t *testing.T) {
	s := openMem(t)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(id string, t time.Time) Row {
		return Row{
			ID: id, OccurredAt: t, Source: "agent-rpc",
			RoverID: "sim-01", Kind: "command", Payload: json.RawMessage(`{}`),
		}
	}
	// 10 rows, recent timestamps, each one minute apart so order is stable.
	for i := 0; i < 10; i++ {
		ts := now.Add(-time.Duration(i) * time.Minute)
		if err := s.Insert(mk("r"+strconv.Itoa(i), ts)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	n, err := s.Sweep(30*24*time.Hour, 5)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 5 {
		t.Errorf("Sweep deleted %d rows, want 5", n)
	}
	got, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	// The five most recent (lowest i) should survive.
	wantIDs := map[string]bool{"r0": true, "r1": true, "r2": true, "r3": true, "r4": true}
	for _, r := range got {
		if !wantIDs[r.ID] {
			t.Errorf("row %q survived but shouldn't have", r.ID)
		}
	}
}

