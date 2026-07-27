package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
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

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
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

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func TestUsers_UpsertFromWorkOS_FirstIsAdmin(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewUsersRepo(pool)

	u1, err := repo.UpsertFromWorkOS(ctx, "wk_1", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !u1.IsAdmin {
		t.Fatal("first user must be admin")
	}

	u2, err := repo.UpsertFromWorkOS(ctx, "wk_2", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u2.IsAdmin {
		t.Fatal("second user must not be admin")
	}

	u2again, err := repo.UpsertFromWorkOS(ctx, "wk_2", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u2again.ID != u2.ID {
		t.Fatal("upsert should be idempotent")
	}
	if u2again.IsAdmin {
		t.Fatal("re-upsert must not promote")
	}

	_ = uuid.UUID{}
}

func TestUsers_List_OrdersByEmail(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewUsersRepo(pool)

	if _, err := repo.UpsertFromWorkOS(ctx, "wk_b", "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertFromWorkOS(ctx, "wk_a", "alice@example.com"); err != nil {
		t.Fatal(err)
	}

	out, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 users, got %d", len(out))
	}
	if out[0].Email != "alice@example.com" || out[1].Email != "bob@example.com" {
		t.Fatalf("expected alice,bob ordering; got %q,%q", out[0].Email, out[1].Email)
	}
	if out[0].LastLoginAt == nil {
		t.Fatal("LastLoginAt should be set after upsert")
	}
}

func TestUsers_List_Empty(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	repo := NewUsersRepo(pool)

	out, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("List should return non-nil slice when empty")
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 users, got %d", len(out))
	}
}

func TestRovers_CreateListUpdateLastSeen(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	r, err := rovers.Create(ctx, "sim-01", "Sim", "PUBKEY1", "", "", admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "sim-01" {
		t.Fatalf("got id %q", r.ID)
	}

	all, err := rovers.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d rovers", len(all))
	}

	if err := rovers.UpdateLastSeen(ctx, "sim-01", time.Now()); err != nil {
		t.Fatal(err)
	}

	got, err := rovers.Get(ctx, "sim-01")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSeenAt == nil {
		t.Fatal("LastSeenAt must be set")
	}
}

func TestAccess_GrantListRoleFor(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUsersRepo(pool)
	rovers := NewRoversRepo(pool)
	access := NewAccessRepo(pool)

	admin, _ := users.UpsertFromWorkOS(ctx, "wk_admin", "a@e.com")
	bob, _ := users.UpsertFromWorkOS(ctx, "wk_bob", "b@e.com")
	_, _ = rovers.Create(ctx, "sim-01", "Sim", "PK1", "", "", admin.ID)

	if err := access.Grant(ctx, bob.ID, "sim-01", "control", admin.ID); err != nil {
		t.Fatal(err)
	}

	role, err := access.RoleFor(ctx, bob.ID, "sim-01")
	if err != nil {
		t.Fatal(err)
	}
	if role != "control" {
		t.Fatalf("got %q", role)
	}

	rs, err := access.ListForUser(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].RoverID != "sim-01" || rs[0].Role != "control" {
		t.Fatalf("got %+v", rs)
	}

	role, err = access.EffectiveRole(ctx, admin.ID, "sim-01", true)
	if err != nil {
		t.Fatal(err)
	}
	if role != "admin" {
		t.Fatalf("admin must default to admin role; got %q", role)
	}

	carol, _ := users.UpsertFromWorkOS(ctx, "wk_carol", "c@e.com")
	role, err = access.EffectiveRole(ctx, carol.ID, "sim-01", false)
	if err != nil {
		t.Fatal(err)
	}
	if role != "" {
		t.Fatalf("expected no role, got %q", role)
	}
}
