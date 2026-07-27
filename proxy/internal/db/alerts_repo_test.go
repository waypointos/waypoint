package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func mkAlertRow(id, rover string, sev string, at time.Time) AlertRow {
	return AlertRow{
		ID:       id,
		RoverID:  rover,
		Severity: sev,
		Source:   "agent.proxy",
		Code:     "proxy_disconnected",
		Message:  "leaf disconnected",
		Metadata: json.RawMessage(`{"hint":"check uplink"}`),
		RaisedAt: at,
	}
}

func TestAlerts_Raise_ListActive(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatalf("upsert admin: %v", err)
	}
	if _, err := rovers.Create(ctx, "sim-01", "Sim 1", "PK1", "", "", admin.ID); err != nil {
		t.Fatalf("create rover: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	a := mkAlertRow("a1", "sim-01", "critical", now)
	if err := repo.Raise(ctx, a); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := repo.ListActive(ctx, "sim-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	r := got[0]
	if r.ID != "a1" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Severity != "critical" {
		t.Errorf("Severity = %q", r.Severity)
	}
	if r.State != "active" {
		t.Errorf("State = %q", r.State)
	}
	if r.RoverID != "sim-01" {
		t.Errorf("RoverID = %q", r.RoverID)
	}
	if string(r.Metadata) == "" {
		t.Errorf("Metadata empty")
	}
	var pm map[string]string
	if err := json.Unmarshal(r.Metadata, &pm); err != nil {
		t.Errorf("metadata not JSON: %v", err)
	}
	if pm["hint"] != "check uplink" {
		t.Errorf("metadata hint = %q", pm["hint"])
	}
}

func TestAlerts_Raise_Idempotent_UpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("dup", "sim-01", "warning", now)); err != nil {
		t.Fatalf("raise 1: %v", err)
	}

	updated := mkAlertRow("dup", "sim-01", "critical", now.Add(5*time.Second))
	updated.Message = "second raise"
	if err := repo.Raise(ctx, updated); err != nil {
		t.Fatalf("raise 2: %v", err)
	}

	got, err := repo.ListActive(ctx, "sim-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	r := got[0]
	if r.Severity != "critical" {
		t.Errorf("Severity = %q, want updated", r.Severity)
	}
	if r.Message != "second raise" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestAlerts_RaiseAfterResolve_Reopens(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("reopen", "sim-01", "critical", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := repo.Resolve(ctx, "reopen", nil, now.Add(time.Minute), true); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	active, err := repo.ListActive(ctx, "sim-01")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active after resolve, got %d", len(active))
	}

	if err := repo.Raise(ctx, mkAlertRow("reopen", "sim-01", "critical", now.Add(2*time.Minute))); err != nil {
		t.Fatalf("re-raise: %v", err)
	}

	active, err = repo.ListActive(ctx, "sim-01")
	if err != nil {
		t.Fatalf("list active after re-raise: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active after re-raise, got %d", len(active))
	}
	if active[0].State != "active" {
		t.Errorf("State = %q, want active", active[0].State)
	}
	if active[0].ResolvedAt != nil {
		t.Errorf("ResolvedAt should be nil, got %v", *active[0].ResolvedAt)
	}
	if active[0].AutoResolved {
		t.Errorf("AutoResolved should have been cleared")
	}
}

func TestAlerts_Acknowledge_Transitions(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("ack-me", "sim-01", "warning", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	ackTime := now.Add(30 * time.Second)
	if err := repo.Acknowledge(ctx, "ack-me", admin.ID, ackTime); err != nil {
		t.Fatalf("ack: %v", err)
	}

	got, err := repo.ListActive(ctx, "sim-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1 (acknowledged is active)", len(got))
	}
	r := got[0]
	if r.State != "acknowledged" {
		t.Errorf("State = %q", r.State)
	}
	if r.AcknowledgedBy == nil || *r.AcknowledgedBy != admin.ID {
		t.Errorf("AcknowledgedBy = %v, want %v", r.AcknowledgedBy, admin.ID)
	}
	if r.AcknowledgedAt == nil {
		t.Errorf("AcknowledgedAt nil")
	}
}

func TestAlerts_Resolve_AutoNoUser(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("auto", "sim-01", "warning", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := repo.Resolve(ctx, "auto", nil, now.Add(time.Minute), true); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	active, _ := repo.ListActive(ctx, "sim-01")
	if len(active) != 0 {
		t.Fatalf("active = %d, want 0", len(active))
	}

	hist, err := repo.GetHistory(ctx, "sim-01", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history = %d, want 1", len(hist))
	}
	r := hist[0]
	if r.State != "resolved" {
		t.Errorf("State = %q", r.State)
	}
	if !r.AutoResolved {
		t.Errorf("AutoResolved = false, want true")
	}
	if r.ResolvedBy != nil {
		t.Errorf("ResolvedBy = %v, want nil for auto-resolve", *r.ResolvedBy)
	}
}

func TestAlerts_Resolve_ByUser(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("by-user", "sim-01", "warning", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := repo.Resolve(ctx, "by-user", &admin.ID, now.Add(time.Minute), false); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	hist, _ := repo.GetHistory(ctx, "sim-01", now.Add(-time.Hour))
	if len(hist) != 1 {
		t.Fatalf("history = %d", len(hist))
	}
	r := hist[0]
	if r.AutoResolved {
		t.Errorf("AutoResolved = true, want false")
	}
	if r.ResolvedBy == nil || *r.ResolvedBy != admin.ID {
		t.Errorf("ResolvedBy = %v, want %v", r.ResolvedBy, admin.ID)
	}
}

func TestAlerts_ListActive_FilterByRover(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim 1", "PK1", "", "", admin.ID)
	_, _ = rovers.Create(ctx, "sim-02", "Sim 2", "PK2", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("a", "sim-01", "info", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := repo.Raise(ctx, mkAlertRow("b", "sim-01", "info", now.Add(time.Second))); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := repo.Raise(ctx, mkAlertRow("c", "sim-02", "info", now.Add(2*time.Second))); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := repo.ListActive(ctx, "sim-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	all, _ := repo.ListActive(ctx, "")
	if len(all) != 3 {
		t.Fatalf("all = %d, want 3", len(all))
	}
	if all[0].ID != "c" {
		t.Errorf("first = %q, want c (newest)", all[0].ID)
	}
}

func TestAlerts_GetHistory_Since(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("old", "sim-01", "info", now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("raise old: %v", err)
	}
	if err := repo.Raise(ctx, mkAlertRow("mid", "sim-01", "info", now.Add(-30*time.Minute))); err != nil {
		t.Fatalf("raise mid: %v", err)
	}
	if err := repo.Raise(ctx, mkAlertRow("new", "sim-01", "info", now)); err != nil {
		t.Fatalf("raise new: %v", err)
	}

	got, err := repo.GetHistory(ctx, "sim-01", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestAlerts_Acknowledge_UnknownIDIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	repo := NewAlertsRepo(pool)
	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")

	err := repo.Acknowledge(ctx, "nope", admin.ID, time.Now())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}

func TestAlerts_Resolve_UnknownIDIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewAlertsRepo(pool)

	err := repo.Resolve(ctx, "nope", nil, time.Now(), true)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}

func TestAlerts_GetByID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.Raise(ctx, mkAlertRow("get-me", "sim-01", "warning", now)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := repo.GetByID(ctx, "get-me")
	if err != nil {
		t.Fatalf("getbyid: %v", err)
	}
	if got.ID != "get-me" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.RoverID != "sim-01" {
		t.Errorf("RoverID = %q", got.RoverID)
	}
	if got.Severity != "warning" {
		t.Errorf("Severity = %q", got.Severity)
	}
	if got.State != "active" {
		t.Errorf("State = %q", got.State)
	}
	if len(got.Metadata) == 0 {
		t.Errorf("Metadata empty")
	}
}

func TestAlerts_GetByID_UnknownIDIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewAlertsRepo(pool)

	_, err := repo.GetByID(ctx, "nope")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}

func TestAlerts_Raise_NoMetadataStoresNull(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	repo := NewAlertsRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	a := mkAlertRow("nometa", "sim-01", "info", time.Now().UTC())
	a.Metadata = nil
	if err := repo.Raise(ctx, a); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, _ := repo.ListActive(ctx, "")
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Metadata != nil {
		t.Errorf("Metadata = %s, want nil", got[0].Metadata)
	}
}
