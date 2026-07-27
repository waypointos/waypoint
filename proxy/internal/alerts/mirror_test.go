package alerts

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func newMirrorTestPool(t *testing.T) *pgxpool.Pool {
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

func startEmbeddedNATS(t *testing.T) (*nats.Conn, func()) {
	t.Helper()
	s, err := natsserver.NewServer(&natsserver.Options{
		Host:           "127.0.0.1",
		Port:           -1,
		NoLog:          true,
		NoSigs:         true,
		MaxControlLine: 4096,
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(2 * time.Second) {
		s.Shutdown()
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		s.Shutdown()
		t.Fatalf("nats connect: %v", err)
	}
	return nc, func() {
		nc.Close()
		s.Shutdown()
		s.WaitForShutdown()
	}
}

// seedAdminAndRover creates a baseline admin user and rover. The alerts table
// references rovers(id) via FK; we must seed before any AlertRaised.
func seedAdminAndRover(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roverID string) uuid.UUID {
	t.Helper()
	users := db.NewUsersRepo(pool)
	rovers := db.NewRoversRepo(pool)
	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatalf("upsert admin: %v", err)
	}
	if _, err := rovers.Create(ctx, roverID, "Sim "+roverID, "PK_"+roverID, "", "", admin.ID); err != nil {
		t.Fatalf("create rover %s: %v", roverID, err)
	}
	return admin.ID
}

func waitForActiveCount(t *testing.T, ctx context.Context, repo *db.AlertsRepo, want int, deadline time.Duration) []db.AlertRow {
	t.Helper()
	var last []db.AlertRow
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		rows, err := repo.ListActive(ctx, "")
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

func waitForState(t *testing.T, ctx context.Context, repo *db.AlertsRepo, id, wantState string, deadline time.Duration) db.AlertRow {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		rows, err := repo.GetHistory(ctx, "", time.Unix(0, 0))
		if err != nil {
			t.Fatalf("history: %v", err)
		}
		for _, r := range rows {
			if r.ID == id && r.State == wantState {
				return r
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("state %q on id %q not observed within %v", wantState, id, deadline)
	return db.AlertRow{}
}

func publishProto(t *testing.T, nc *nats.Conn, subj string, m proto.Message) {
	t.Helper()
	body, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := nc.Publish(subj, body); err != nil {
		t.Fatalf("publish %s: %v", subj, err)
	}
}

func TestMirror_RaiseAckResolveFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newMirrorTestPool(t)
	repo := db.NewAlertsRepo(pool)
	adminID := seedAdminAndRover(t, ctx, pool, "sim-01")

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := StartMirror(ctx, nc, repo); err != nil {
		t.Fatalf("StartMirror: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// 1. Raise.
	id := "sim-01:agent.proxy:proxy_disconnected"
	publishProto(t, nc, "waypoint.sim-01.alert.raised", &waypointv1.AlertRaised{
		T:        timestamppb.Now(),
		Id:       id,
		Source:   "agent.proxy",
		Severity: waypointv1.Severity_SEVERITY_FAULT,
		Code:     "proxy_disconnected",
		Message:  "leaf disconnected",
		Metadata: map[string]string{"hint": "check uplink"},
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush raise: %v", err)
	}

	rows := waitForActiveCount(t, ctx, repo, 1, 3*time.Second)
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.ID != id {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", r.Severity)
	}
	if r.State != "active" {
		t.Errorf("State = %q", r.State)
	}
	if r.RoverID != "sim-01" {
		t.Errorf("RoverID = %q", r.RoverID)
	}

	// 2. Acknowledge.
	publishProto(t, nc, "waypoint.sim-01.alert.acknowledged", &waypointv1.AlertAcknowledged{
		T:      timestamppb.Now(),
		Id:     id,
		UserId: adminID.String(),
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush ack: %v", err)
	}
	acked := waitForState(t, ctx, repo, id, "acknowledged", 3*time.Second)
	if acked.AcknowledgedBy == nil || *acked.AcknowledgedBy != adminID {
		t.Errorf("AcknowledgedBy = %v, want %v", acked.AcknowledgedBy, adminID)
	}

	// 3. Auto-resolve (no user).
	publishProto(t, nc, "waypoint.sim-01.alert.resolved", &waypointv1.AlertResolved{
		T:            timestamppb.Now(),
		Id:           id,
		Reason:       "leaf reconnected",
		AutoResolved: true,
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush resolve: %v", err)
	}
	resolved := waitForState(t, ctx, repo, id, "resolved", 3*time.Second)
	if !resolved.AutoResolved {
		t.Errorf("AutoResolved = false, want true")
	}
	if resolved.ResolvedBy != nil {
		t.Errorf("ResolvedBy = %v, want nil for auto-resolve", *resolved.ResolvedBy)
	}

	active, _ := repo.ListActive(ctx, "")
	if len(active) != 0 {
		t.Errorf("active rows after resolve = %d, want 0", len(active))
	}
}

func TestMirror_ResolveByUser_FromReason(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newMirrorTestPool(t)
	repo := db.NewAlertsRepo(pool)
	adminID := seedAdminAndRover(t, ctx, pool, "sim-01")

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := StartMirror(ctx, nc, repo); err != nil {
		t.Fatalf("StartMirror: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	id := "sim-01:agent.test:by_user"
	publishProto(t, nc, "waypoint.sim-01.alert.raised", &waypointv1.AlertRaised{
		T:        timestamppb.Now(),
		Id:       id,
		Source:   "agent.test",
		Severity: waypointv1.Severity_SEVERITY_WARN,
		Code:     "by_user",
		Message:  "n/a",
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	waitForActiveCount(t, ctx, repo, 1, 3*time.Second)

	publishProto(t, nc, "waypoint.sim-01.alert.resolved", &waypointv1.AlertResolved{
		T:            timestamppb.Now(),
		Id:           id,
		Reason:       "user:" + adminID.String(),
		AutoResolved: false,
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush resolve: %v", err)
	}

	r := waitForState(t, ctx, repo, id, "resolved", 3*time.Second)
	if r.AutoResolved {
		t.Errorf("AutoResolved = true, want false")
	}
	if r.ResolvedBy == nil || *r.ResolvedBy != adminID {
		t.Errorf("ResolvedBy = %v, want %v", r.ResolvedBy, adminID)
	}
}

func TestMirror_AckWithEmptyUserIsDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newMirrorTestPool(t)
	repo := db.NewAlertsRepo(pool)
	seedAdminAndRover(t, ctx, pool, "sim-01")

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := StartMirror(ctx, nc, repo); err != nil {
		t.Fatalf("StartMirror: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	id := "sim-01:agent.test:bad_ack"
	publishProto(t, nc, "waypoint.sim-01.alert.raised", &waypointv1.AlertRaised{
		T:        timestamppb.Now(),
		Id:       id,
		Source:   "agent.test",
		Severity: waypointv1.Severity_SEVERITY_INFO,
		Code:     "bad_ack",
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	waitForActiveCount(t, ctx, repo, 1, 3*time.Second)

	publishProto(t, nc, "waypoint.sim-01.alert.acknowledged", &waypointv1.AlertAcknowledged{
		T:      timestamppb.Now(),
		Id:     id,
		UserId: "", // empty → dropped
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush ack: %v", err)
	}

	// Give the mirror time to process (and drop).
	time.Sleep(200 * time.Millisecond)
	rows, _ := repo.ListActive(ctx, "")
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1 (ack should be dropped)", len(rows))
	}
	if rows[0].State != "active" {
		t.Errorf("State = %q, want active (ack should not have applied)", rows[0].State)
	}
}

func TestMirror_RaiseForUnknownRoverIsLoggedNotPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newMirrorTestPool(t)
	repo := db.NewAlertsRepo(pool)
	// Note: no rover seeded — FK will reject the insert.

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := StartMirror(ctx, nc, repo); err != nil {
		t.Fatalf("StartMirror: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	publishProto(t, nc, "waypoint.ghost-rover.alert.raised", &waypointv1.AlertRaised{
		T:        timestamppb.Now(),
		Id:       "ghost-1",
		Source:   "agent.test",
		Severity: waypointv1.Severity_SEVERITY_INFO,
		Code:     "ghost",
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// 200ms is enough for the subscription to attempt the insert and fail.
	// The test just verifies we don't crash and that nothing got persisted.
	time.Sleep(200 * time.Millisecond)
	rows, err := repo.ListActive(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0 (FK should have rejected)", len(rows))
	}
}

func TestMirror_IgnoresMalformedProto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newMirrorTestPool(t)
	repo := db.NewAlertsRepo(pool)
	seedAdminAndRover(t, ctx, pool, "sim-01")

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := StartMirror(ctx, nc, repo); err != nil {
		t.Fatalf("StartMirror: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	for _, subj := range []string{
		"waypoint.sim-01.alert.raised",
		"waypoint.sim-01.alert.acknowledged",
		"waypoint.sim-01.alert.resolved",
	} {
		if err := nc.Publish(subj, []byte{0xff, 0xff, 0xff}); err != nil {
			t.Fatalf("publish %s: %v", subj, err)
		}
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	rows, _ := repo.ListActive(ctx, "")
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestMirror_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := newMirrorTestPool(t)
	repo := db.NewAlertsRepo(pool)
	seedAdminAndRover(t, ctx, pool, "sim-01")

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	if err := StartMirror(ctx, nc, repo); err != nil {
		t.Fatalf("StartMirror: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	publishProto(t, nc, "waypoint.sim-01.alert.raised", &waypointv1.AlertRaised{
		T:        timestamppb.Now(),
		Id:       "after-cancel",
		Source:   "agent.test",
		Severity: waypointv1.Severity_SEVERITY_INFO,
		Code:     "ignored",
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	rows, err := repo.ListActive(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestExtractRoverID_Mirror(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"waypoint.sim-01.alert.raised", "sim-01"},
		{"waypoint.sim-01.alert.acknowledged", "sim-01"},
		{"waypoint.x.alert.resolved", "x"},
		{"notwaypoint.sim-01.alert.raised", ""},
		{"waypoint", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := extractRoverID(c.in)
		if got != c.want {
			t.Errorf("extractRoverID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSeverityToString_Mirror(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   waypointv1.Severity
		want string
	}{
		{waypointv1.Severity_SEVERITY_UNSPECIFIED, "info"},
		{waypointv1.Severity_SEVERITY_INFO, "info"},
		{waypointv1.Severity_SEVERITY_WARN, "warning"},
		{waypointv1.Severity_SEVERITY_FAULT, "critical"},
	}
	for _, c := range cases {
		got := severityToString(c.in)
		if got != c.want {
			t.Errorf("severityToString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseUserFromReason(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	cases := []struct {
		in     string
		wantOK bool
		want   uuid.UUID
	}{
		{"user:" + id.String(), true, id},
		{"user:not-a-uuid", false, uuid.UUID{}},
		{"user:", false, uuid.UUID{}},
		{"auto", false, uuid.UUID{}},
		{"", false, uuid.UUID{}},
	}
	for _, c := range cases {
		got, ok := parseUserFromReason(c.in)
		if ok != c.wantOK {
			t.Errorf("parseUserFromReason(%q) ok = %v, want %v", c.in, ok, c.wantOK)
		}
		if ok && got != c.want {
			t.Errorf("parseUserFromReason(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
