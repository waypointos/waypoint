package authkit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/jwt/v2"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
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

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// callNatsCred wires the handler behind RequireUser and returns the recorder.
func callNatsCred(t *testing.T, deps *Deps, sessionUserID string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("POST /api/auth/nats-cred",
		RequireUser(deps.Session, deps.Users, http.HandlerFunc(deps.NatsCred)))

	rec := httptest.NewRecorder()
	if err := deps.Session.Set(rec, Session{UserID: sessionUserID}); err != nil {
		t.Fatalf("session set: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/auth/nats-cred", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	out := httptest.NewRecorder()
	mux.ServeHTTP(out, req)
	return out
}

func contains(list jwt.StringList, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestNatsCred_NonAdminUserGetsRoleScopedJWT(t *testing.T) {
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
	if _, err := rovers.Create(ctx, "sim-02", "Sim 2", "PK2", "", "", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err := access.Grant(ctx, bob.ID, "sim-01", "control", admin.ID); err != nil {
		t.Fatal(err)
	}

	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}

	deps := &Deps{
		Users:         users,
		Session:       NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false),
		Access:        access,
		Rovers:        rovers,
		Operator:      op,
		PublicNATSURL: "/ws/nats",
	}

	rec := callNatsCred(t, deps, bob.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for _, key := range []string{"jwt", "seed", "expiresAt", "natsUrl"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("response missing %q: %v", key, body)
		}
	}
	if body["natsUrl"] != "/ws/nats" {
		t.Fatalf("natsUrl=%v", body["natsUrl"])
	}

	tok, _ := body["jwt"].(string)
	seedB64, _ := body["seed"].(string)
	if tok == "" || seedB64 == "" {
		t.Fatalf("empty jwt or seed")
	}
	if _, err := base64.StdEncoding.DecodeString(seedB64); err != nil {
		t.Fatalf("seed not base64: %v", err)
	}

	claims, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatalf("decode jwt: %v", err)
	}
	// Bob has control on sim-01 only.
	if !contains(claims.Permissions.Pub.Allow, "waypoint.sim-01.cmd.>") {
		t.Fatalf("missing pub waypoint.sim-01.cmd.>: %+v", claims.Permissions.Pub.Allow)
	}
	// Sub allow is the rover's full namespace as a single wildcard —
	// matches the rover account's Stream Export so the dashboard can
	// subscribe to any leaf subject (telemetry, event, alert, infra,
	// module, diag, etc.) without per-namespace enumeration in the JWT.
	if !contains(claims.Permissions.Sub.Allow, "waypoint.sim-01.>") {
		t.Fatalf("missing sub waypoint.sim-01.>: %+v", claims.Permissions.Sub.Allow)
	}
	// Bob has NO access to sim-02 even though it exists.
	if contains(claims.Permissions.Pub.Allow, "waypoint.sim-02.cmd.>") {
		t.Fatalf("bob must not get sim-02 pub")
	}
	if contains(claims.Permissions.Sub.Allow, "waypoint.sim-02.>") {
		t.Fatalf("bob must not get sim-02 sub")
	}
}

func TestNatsCred_AdminCoversAllRovers(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	rovers := db.NewRoversRepo(pool)
	access := db.NewAccessRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !admin.IsAdmin {
		t.Fatal("first user must be admin")
	}
	if _, err := rovers.Create(ctx, "sim-01", "Sim 1", "PK1", "", "", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rovers.Create(ctx, "sim-02", "Sim 2", "PK2", "", "", admin.ID); err != nil {
		t.Fatal(err)
	}

	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}

	deps := &Deps{
		Users:         users,
		Session:       NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false),
		Access:        access,
		Rovers:        rovers,
		Operator:      op,
		PublicNATSURL: "/ws/nats",
	}

	rec := callNatsCred(t, deps, admin.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	tok, _ := body["jwt"].(string)
	claims, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatalf("decode jwt: %v", err)
	}

	// Admin gets rpc + cmd on every rover.
	for _, rover := range []string{"sim-01", "sim-02"} {
		if !contains(claims.Permissions.Pub.Allow, "waypoint."+rover+".cmd.>") {
			t.Fatalf("admin missing pub waypoint.%s.cmd.>", rover)
		}
		if !contains(claims.Permissions.Pub.Allow, "waypoint."+rover+".rpc.>") {
			t.Fatalf("admin missing pub waypoint.%s.rpc.>", rover)
		}
		if !contains(claims.Permissions.Sub.Allow, "waypoint."+rover+".>") {
			t.Fatalf("admin missing sub waypoint.%s.>", rover)
		}
	}
	if !contains(claims.Permissions.Pub.Allow, "_INBOX.>") {
		t.Fatal("admin missing _INBOX.> pub")
	}
}

func TestNatsCred_NoAccessReturns200WithEmptyPerms(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	users := db.NewUsersRepo(pool)
	rovers := db.NewRoversRepo(pool)
	access := db.NewAccessRepo(pool)

	admin, err := users.UpsertFromWorkOS(ctx, "wk_admin", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Bob is not admin and has no grants.
	bob, err := users.UpsertFromWorkOS(ctx, "wk_bob", "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if bob.IsAdmin {
		t.Fatal("bob must not be admin")
	}
	if _, err := rovers.Create(ctx, "sim-01", "Sim 1", "PK1", "", "", admin.ID); err != nil {
		t.Fatal(err)
	}

	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Users:         users,
		Session:       NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false),
		Access:        access,
		Rovers:        rovers,
		Operator:      op,
		PublicNATSURL: "/ws/nats",
	}

	rec := callNatsCred(t, deps, bob.ID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tok, _ := body["jwt"].(string)
	claims, err := jwt.DecodeUserClaims(tok)
	if err != nil {
		t.Fatalf("decode jwt: %v", err)
	}
	if len(claims.Permissions.Pub.Allow) != 0 || len(claims.Permissions.Sub.Allow) != 0 {
		t.Fatalf("expected empty perms, got pub=%v sub=%v",
			claims.Permissions.Pub.Allow, claims.Permissions.Sub.Allow)
	}
}

func TestNatsCred_Unauthorized(t *testing.T) {
	// No session cookie at all → 401 from RequireUser; the handler is not reached.
	deps := &Deps{
		Session: NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false),
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/auth/nats-cred",
		RequireUser(deps.Session, nil, http.HandlerFunc(deps.NatsCred)))

	req := httptest.NewRequest("POST", "/api/auth/nats-cred", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}
