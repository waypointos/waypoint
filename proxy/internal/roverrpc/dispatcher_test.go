package roverrpc_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/waypointos/waypoint/proxy/internal/natshub"
	"github.com/waypointos/waypoint/proxy/internal/operator"
	"github.com/waypointos/waypoint/proxy/internal/roverconn"
	"github.com/waypointos/waypoint/proxy/internal/roverrpc"
)

// echoHandler answers a fixed leaf subject with a sentinel payload. Used to
// isolate dispatcher wiring from the basemap or ice_servers response shapes.
type echoHandler struct {
	leaf    string
	payload []byte
	calls   chan []byte
}

func (h *echoHandler) Leaf() string { return h.leaf }
func (h *echoHandler) Handle(req []byte) []byte {
	if h.calls != nil {
		h.calls <- append([]byte(nil), req...)
	}
	return h.payload
}

// stubConns implements the dispatcher's conns interface from a static map so
// the unit tests don't need a real NATS server.
type stubConns struct {
	conns map[string]*nats.Conn
	err   error
}

func (s *stubConns) Conn(roverID string) (*nats.Conn, error) {
	if s.err != nil {
		return nil, s.err
	}
	if c, ok := s.conns[roverID]; ok {
		return c, nil
	}
	return nil, errors.New("no conn for " + roverID)
}

func TestDispatcher_EnsureIsIdempotent(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	d := roverrpc.New(&stubConns{conns: map[string]*nats.Conn{"r1": nc}},
		&echoHandler{leaf: "rpc.ping", payload: []byte("pong")})

	if err := d.Ensure("r1"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if err := d.Ensure("r1"); err != nil {
		t.Fatalf("second ensure (should be no-op): %v", err)
	}

	resp, err := nc.Request("waypoint.r1.rpc.ping", nil, time.Second)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if string(resp.Data) != "pong" {
		t.Fatalf("payload: %q", resp.Data)
	}
}

func TestDispatcher_RemoveUnsubscribes(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	d := roverrpc.New(&stubConns{conns: map[string]*nats.Conn{"r1": nc}},
		&echoHandler{leaf: "rpc.ping", payload: []byte("pong")})

	if err := d.Ensure("r1"); err != nil {
		t.Fatal(err)
	}
	d.Remove("r1")

	if _, err := nc.Request("waypoint.r1.rpc.ping", nil, 200*time.Millisecond); err == nil {
		t.Fatal("expected timeout after Remove")
	}
}

// TestDispatcher_LeafBackedRoverGetsReply is the regression test for the bug
// that motivated this package. A leaf-backed rover publishes
// waypoint.<id>.rpc.basemap_tile in its own account; before the dispatcher
// existed, the proxy responder lived in the sessions account behind a Stream
// import that delivers the request but cannot route the reply back to a
// leaf-backed publisher. The dispatcher pins the responder onto the rover's
// own-account connection so reply routing stays intra-account.
func TestDispatcher_LeafBackedRoverGetsReply(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := natshub.Start(context.Background(), natshub.Config{
		Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-roverrpc"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(roverAcctJWT); err != nil {
		t.Fatal(err)
	}
	sessAcctJWT, err := op.MintSessionAccountJWT([]string{"waypoint.>"},
		[]operator.RoverImport{{RoverID: roverID, AccountPubKey: roverAcctPK}})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(sessAcctJWT); err != nil {
		t.Fatal(err)
	}

	// Stand up a leaf-backed agent NATS server (matches the rover deployment).
	mux := http.NewServeMux()
	mux.Handle("GET /leaf/", http.StripPrefix("/leaf", h.LeafHandler()))
	publicSrv := httptest.NewServer(mux)
	defer publicSrv.Close()
	credsPath := writeCreds(t, roverUserJWT, string(roverUserSeed))
	defer os.Remove(credsPath)
	leafURL := strings.Replace(publicSrv.URL, "http://", "ws://", 1) + "/leaf"
	u, err := url.Parse(leafURL)
	if err != nil {
		t.Fatal(err)
	}
	agentOpts := &server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, MaxControlLine: 4096}
	agentOpts.LeafNode.Remotes = []*server.RemoteLeafOpts{{URLs: []*url.URL{u}, Credentials: credsPath}}
	agentOpts.LeafNode.ReconnectInterval = 250 * time.Millisecond
	agentSrv, err := server.NewServer(agentOpts)
	if err != nil {
		t.Fatal(err)
	}
	go agentSrv.Start()
	defer agentSrv.Shutdown()
	if !agentSrv.ReadyForConnections(2 * time.Second) {
		t.Fatal("agent nats not ready")
	}
	deadline := time.After(5 * time.Second)
	for h.Server.NumLeafNodes() == 0 {
		select {
		case <-deadline:
			t.Fatal("leaf never connected")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Dispatcher running on the proxy side, using the rover-account pool.
	pool := roverconn.NewPool(h.Server.ClientURL(), func(id string) (string, string, error) {
		j, s, err := op.MintRoverInternalUserJWT(roverAcctPK, id)
		return j, string(s), err
	})
	defer pool.Close()
	d := roverrpc.New(pool, &echoHandler{leaf: "rpc.basemap_tile", payload: []byte("TILE")})
	if err := d.Ensure(roverID); err != nil {
		t.Fatalf("dispatcher ensure: %v", err)
	}

	// Agent-side client: publish the request from inside the rover's account
	// across the leaf, the way the on-rover agent does in production.
	agentNC, err := nats.Connect(agentSrv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer agentNC.Close()

	// The dispatcher's subscription interest crosses the leaf connection
	// asynchronously; retry no-responders until it propagates.
	var resp *nats.Msg
	requestDeadline := time.Now().Add(5 * time.Second)
	for {
		resp, err = agentNC.Request("waypoint."+roverID+".rpc.basemap_tile", []byte("REQ"), 3*time.Second)
		if !errors.Is(err, nats.ErrNoResponders) || time.Now().After(requestDeadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("rover->proxy request failed (the original bug): %v", err)
	}
	if string(resp.Data) != "TILE" {
		t.Fatalf("payload: %q want TILE", resp.Data)
	}
}

// writeCreds is the same shape natsleaf.AttachToOptions uses on the agent and
// mirrors what cmdrelay_test.go does. Kept here so this test doesn't reach into
// another package's testdata.
func writeCreds(t *testing.T, jwt, seed string) string {
	t.Helper()
	f, err := os.CreateTemp("", "roverrpc-leaf-*.creds")
	if err != nil {
		t.Fatal(err)
	}
	body := "-----BEGIN NATS USER JWT-----\n" + jwt + "\n------END NATS USER JWT------\n\n" +
		"-----BEGIN USER NKEY SEED-----\n" + seed + "\n------END USER NKEY SEED------\n"
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
