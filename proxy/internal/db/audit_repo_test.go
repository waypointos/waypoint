package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAudit_InsertList_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAuditRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatalf("upsert admin: %v", err)
	}
	if _, err := rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID); err != nil {
		t.Fatalf("create rover: %v", err)
	}

	row := AuditRow{
		ID:          uuid.New(),
		OccurredAt:  time.Now().UTC().Truncate(time.Microsecond),
		Source:      "proxy-ws",
		ActorUserID: &admin.ID,
		ActorEmail:  "admin@example.com",
		RoverID:     "sim-01",
		Kind:        "command",
		Payload:     json.RawMessage(`{"subject":"waypoint.sim-01.cmd.drive","payload_bytes":7}`),
	}
	if err := repo.Insert(ctx, row); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := repo.List(ctx, AuditFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	r := got[0]
	if r.ID != row.ID {
		t.Errorf("ID = %v, want %v", r.ID, row.ID)
	}
	if r.Source != row.Source {
		t.Errorf("Source = %q, want %q", r.Source, row.Source)
	}
	if r.ActorUserID == nil || *r.ActorUserID != *row.ActorUserID {
		t.Errorf("ActorUserID = %v, want %v", r.ActorUserID, row.ActorUserID)
	}
	if r.ActorEmail != row.ActorEmail {
		t.Errorf("ActorEmail = %q, want %q", r.ActorEmail, row.ActorEmail)
	}
	if r.RoverID != row.RoverID {
		t.Errorf("RoverID = %q, want %q", r.RoverID, row.RoverID)
	}
	if r.Kind != row.Kind {
		t.Errorf("Kind = %q, want %q", r.Kind, row.Kind)
	}
	if string(r.Payload) == "" {
		t.Errorf("Payload empty")
	}
	// Validate JSON survives round-trip.
	var pm map[string]any
	if err := json.Unmarshal(r.Payload, &pm); err != nil {
		t.Errorf("payload not valid JSON: %v", err)
	}
	if pm["subject"] != "waypoint.sim-01.cmd.drive" {
		t.Errorf("payload subject = %v", pm["subject"])
	}
}

func TestAudit_List_FilterByRover(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAuditRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim 1", "PK1", "", "", admin.ID)
	_, _ = rovers.Create(ctx, "sim-02", "Sim 2", "PK2", "", "", admin.ID)

	for i, rid := range []string{"sim-01", "sim-02", "sim-01"} {
		err := repo.Insert(ctx, AuditRow{
			ID:         uuid.New(),
			OccurredAt: time.Now().UTC().Add(time.Duration(-i) * time.Second),
			Source:     "proxy-ws",
			RoverID:    rid,
			Kind:       "command",
			Payload:    json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	got, err := repo.List(ctx, AuditFilter{RoverID: "sim-01"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	for _, r := range got {
		if r.RoverID != "sim-01" {
			t.Errorf("RoverID = %q", r.RoverID)
		}
	}
}

func TestAudit_List_FilterBySince(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAuditRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-time.Hour)
	recent := now.Add(-time.Minute)

	for _, ts := range []time.Time{old, recent, now} {
		err := repo.Insert(ctx, AuditRow{
			ID:         uuid.New(),
			OccurredAt: ts,
			Source:     "proxy-ws",
			RoverID:    "sim-01",
			Kind:       "command",
			Payload:    json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	cutoff := now.Add(-5 * time.Minute)
	got, err := repo.List(ctx, AuditFilter{SinceTime: &cutoff})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (recent + now)", len(got))
	}
	for _, r := range got {
		if r.OccurredAt.Before(cutoff) {
			t.Errorf("row %v is before cutoff %v", r.OccurredAt, cutoff)
		}
	}
}

func TestAudit_List_LimitCaps(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAuditRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	for i := 0; i < 5; i++ {
		err := repo.Insert(ctx, AuditRow{
			ID:         uuid.New(),
			OccurredAt: time.Now().UTC().Add(time.Duration(-i) * time.Second),
			Source:     "proxy-ws",
			RoverID:    "sim-01",
			Kind:       "command",
			Payload:    json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	got, err := repo.List(ctx, AuditFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
}

func TestAudit_Insert_DuplicateIDIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAuditRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	id := uuid.New()
	row := AuditRow{
		ID:         id,
		OccurredAt: time.Now().UTC(),
		Source:     "proxy-ws",
		RoverID:    "sim-01",
		Kind:       "command",
		Payload:    json.RawMessage(`{}`),
	}
	if err := repo.Insert(ctx, row); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	// Re-insert with the same ID but different payload — should be a no-op.
	row.Payload = json.RawMessage(`{"changed":true}`)
	if err := repo.Insert(ctx, row); err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	got, err := repo.List(ctx, AuditFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	// The first payload wins (ON CONFLICT DO NOTHING).
	var pm map[string]any
	_ = json.Unmarshal(got[0].Payload, &pm)
	if _, exists := pm["changed"]; exists {
		t.Errorf("second insert overwrote the first; payload = %s", got[0].Payload)
	}
}
