package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/waypointos/waypoint/proxy/internal/db"
)

func newSubTestPool(t *testing.T) *pgxpool.Pool {
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

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// waitForRows polls AuditRepo.List until it returns at least want rows or the
// deadline expires. Returns the most recent List result.
func waitForRows(t *testing.T, ctx context.Context, repo *db.AuditRepo, want int, deadline time.Duration) []db.AuditRow {
	t.Helper()
	var last []db.AuditRow
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		rows, err := repo.List(ctx, db.AuditFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		last = rows
		if len(rows) >= want {
			return rows
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last
}

func TestSubscriber_PersistsCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newSubTestPool(t)
	repo := db.NewAuditRepo(pool)

	// The audit row references users(id) via FK; create a user so the
	// publisher's actor.ID points at a real row.
	users := db.NewUsersRepo(pool)
	user, err := users.UpsertFromWorkOS(ctx, "wk_alice", "alice@example.com")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := Start(ctx, nc, repo); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Allow the subscription to register on the server side.
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	actor := Actor{ID: user.ID.String(), Email: "alice@example.com"}
	p := NewPublisher(nc, actor, "proxy-ws")
	const subject = "waypoint.sim-01.cmd.drive"
	p.LogCommand(subject, 42)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	rows := waitForRows(t, ctx, repo, 1, 2*time.Second)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Kind != "command" {
		t.Errorf("Kind = %q, want %q", r.Kind, "command")
	}
	if r.RoverID != "sim-01" {
		t.Errorf("RoverID = %q", r.RoverID)
	}
	if r.Source != "proxy-ws" {
		t.Errorf("Source = %q", r.Source)
	}
	if r.ActorEmail != "alice@example.com" {
		t.Errorf("ActorEmail = %q", r.ActorEmail)
	}
	if r.ActorUserID == nil {
		t.Errorf("ActorUserID is nil; want %s", actor.ID)
	} else if r.ActorUserID.String() != actor.ID {
		t.Errorf("ActorUserID = %s, want %s", r.ActorUserID, actor.ID)
	}

	var payload map[string]any
	if err := json.Unmarshal(r.Payload, &payload); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if payload["subject"] != subject {
		t.Errorf("payload subject = %v, want %q", payload["subject"], subject)
	}
	// JSON numbers decode as float64.
	if bytes, ok := payload["payload_bytes"].(float64); !ok || int(bytes) != 42 {
		t.Errorf("payload payload_bytes = %v (%T), want 42", payload["payload_bytes"], payload["payload_bytes"])
	}
}

func TestSubscriber_IgnoresMalformedProto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newSubTestPool(t)
	repo := db.NewAuditRepo(pool)

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := Start(ctx, nc, repo); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Random bytes that are NOT a valid AuditEvent proto.
	if err := nc.Publish("waypoint.sim-01.infra.audit", []byte{0xff, 0xff, 0xff, 0xff, 0xff}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Give the subscriber a chance to (not) insert.
	time.Sleep(200 * time.Millisecond)
	rows, err := repo.List(ctx, db.AuditFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestSubscriber_EmptyActorUserIDStaysNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newSubTestPool(t)
	repo := db.NewAuditRepo(pool)

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := Start(ctx, nc, repo); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// System-issued event: Actor.ID == "" — e.g. agent emitting ImageApplied
	// on its own behalf.
	p := NewPublisher(nc, Actor{ID: "", Email: ""}, "agent-rpc")
	p.LogImageApplied("sim-01", "1.0.0", "1.1.0", "https://example.com/img")
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	rows := waitForRows(t, ctx, repo, 1, 2*time.Second)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.ActorUserID != nil {
		t.Errorf("ActorUserID = %v, want nil for system-issued event", r.ActorUserID)
	}
	if r.Kind != "image" {
		t.Errorf("Kind = %q, want %q", r.Kind, "image")
	}
}

func TestSubscriber_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := newSubTestPool(t)
	repo := db.NewAuditRepo(pool)

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := Start(ctx, nc, repo); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Cancel before publishing — give the cleanup goroutine a moment to
	// unsubscribe.
	cancel()
	time.Sleep(100 * time.Millisecond)

	p := NewPublisher(nc, Actor{ID: uuid.NewString(), Email: "bob@example.com"}, "proxy-ws")
	p.LogCommand("waypoint.sim-01.cmd.drive", 1)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	// repo.List uses the canceled ctx; use a fresh one to query.
	rows, err := repo.List(context.Background(), db.AuditFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows after cancel, want 0", len(rows))
	}
}
