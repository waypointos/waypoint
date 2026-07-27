package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/natshub"
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

// TestGateway_PublishDeniedSurfacesErrEnvelope is the integration test for the
// NATS-native ACL: a monitor user MUST NOT be able to publish on cmd.>, and the
// NATS server's permissions-violation -ERR must surface to the WS client as an
// {Action: "err"} envelope.
func TestGateway_PublishDeniedSurfacesErrEnvelope(t *testing.T) {
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
	if err := access.Grant(ctx, bob.ID, "sim-01", "monitor", admin.ID); err != nil {
		t.Fatal(err)
	}

	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}

	hub, err := natshub.Start(ctx, natshub.Config{
		Operator:    op,
		ClientPort:  -1,
		LeafWSPort:  -1,
		HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Shutdown()
	if !hub.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats hub not ready")
	}

	// Register the sessions account so session user JWTs minted under it
	// validate against the embedded NATS server's account resolver.
	sessAcctJWT, err := op.MintSessionAccountJWT([]string{"waypoint.>"}, nil)
	if err != nil {
		t.Fatalf("mint sessions account: %v", err)
	}
	if err := hub.RegisterAccount(sessAcctJWT); err != nil {
		t.Fatalf("register sessions account: %v", err)
	}

	gw := New(Deps{
		Access:   access,
		Rovers:   rovers,
		Operator: op,
		NATSURL:  hub.Server.ClientURL(),
	})

	session := authkit.NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)
	mux := http.NewServeMux()
	mux.Handle("GET /ws/nats",
		authkit.RequireUser(session, users, http.HandlerFunc(gw.Handle)))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Bake a session cookie for bob.
	rec := httptest.NewRecorder()
	if err := session.Set(rec, authkit.Session{UserID: bob.ID.String()}); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("session cookie not set")
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/nats"
	// Build a Cookie header manually — the gorilla dialer doesn't carry an
	// http.CookieJar like net/http does.
	header := http.Header{}
	var cookieHdr strings.Builder
	for i, c := range cookies {
		if i > 0 {
			cookieHdr.WriteString("; ")
		}
		cookieHdr.WriteString(c.Name)
		cookieHdr.WriteString("=")
		cookieHdr.WriteString(c.Value)
	}
	header.Set("Cookie", cookieHdr.String())

	c, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("ws dial: %v (status %d)", err, resp.StatusCode)
		}
		t.Fatalf("ws dial: %v", err)
	}
	defer c.Close()

	// Bob has monitor on sim-01; cmd.> publish is forbidden by the NATS
	// permissions on his session JWT. The server replies with an -ERR which
	// nats.go surfaces via the connection's ErrorHandler — the gateway must
	// translate that into an {Action: "err"} envelope.
	if err := c.WriteJSON(envelope{
		Action:  "pub",
		Subject: "waypoint.sim-01.cmd.drive",
		Data:    base64.StdEncoding.EncodeToString([]byte("x")),
	}); err != nil {
		t.Fatalf("write pub: %v", err)
	}

	if err := c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, raw, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read err envelope: %v", err)
	}
	var got envelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if got.Action != "err" {
		t.Fatalf("expected Action=err, got %+v", got)
	}
	body, err := base64.StdEncoding.DecodeString(got.Data)
	if err != nil {
		t.Fatalf("decode err body: %v", err)
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "permission") && !strings.Contains(lower, "authorization") {
		t.Fatalf("err body should mention permission/authorization, got %q", string(body))
	}
}
