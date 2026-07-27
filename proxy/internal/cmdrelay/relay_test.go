package cmdrelay_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/waypointos/waypoint/proxy/internal/cmdrelay"
	"github.com/waypointos/waypoint/proxy/internal/natshub"
	"github.com/waypointos/waypoint/proxy/internal/operator"
	"github.com/waypointos/waypoint/proxy/internal/roverconn"
)

func writeCreds(t *testing.T, jwt, seed string) string {
	t.Helper()
	f, err := os.CreateTemp("", "cmdrelay-leaf-*.creds")
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

// TestRelay_SessionsCmdReachesRoverBehindLeaf proves the relay path end-to-end:
// a sessions-account control session publishes cmd.drive; the relay forwards it
// on the rover's own-account connection; the leaf carries it to a leaf-backed
// subscriber (core stand-in).
func TestRelay_SessionsCmdReachesRoverBehindLeaf(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := natshub.Start(context.Background(), natshub.Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1, Logs: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-cmd-relay"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, nil) // inherit
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

	// Agent embedded server joined via leaf; core stand-in subscribes cmd.drive.
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
	agentNC, err := nats.Connect(agentSrv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer agentNC.Close()
	subj := "waypoint." + roverID + ".cmd.drive"
	sub, err := agentNC.SubscribeSync(subj)
	if err != nil {
		t.Fatal(err)
	}
	if err := agentNC.Flush(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let sub interest propagate up the leaf

	// Relay: subscribes on the sessions-account internal connection, republishes
	// on the rover-account connection pool.
	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}
	relayNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(internalJWT, string(internalSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer relayNC.Close()
	pool := roverconn.NewPool(h.Server.ClientURL(), func(id string) (string, string, error) {
		j, s, err := op.MintRoverInternalUserJWT(roverAcctPK, id)
		return j, string(s), err
	})
	defer pool.Close()
	if err := cmdrelay.Start(relayNC, pool); err != nil {
		t.Fatalf("relay start: %v", err)
	}
	if err := relayNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Publisher: sessions-account control session.
	sessUserJWT, sessUserSeed, err := op.MintSessionUserJWT("u1",
		[]operator.SessionAccess{{RoverID: roverID, Role: "control"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	pubNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(sessUserJWT, string(sessUserSeed)))
	if err != nil {
		t.Fatalf("sessions publisher connect: %v", err)
	}
	defer pubNC.Close()
	if err := pubNC.Publish(subj, []byte("drive")); err != nil {
		t.Fatal(err)
	}
	if err := pubNC.Flush(); err != nil {
		t.Fatal(err)
	}

	msg, err := sub.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("cmd did not reach leaf-backed subscriber via relay: %v", err)
	}
	if string(msg.Data) != "drive" {
		t.Fatalf("payload mismatch: %q", msg.Data)
	}
}

// TestRelay_ForwardsModuleCommand proves an interactive panel's command frame
// (module.*.command) relays browser->rover the same way cmd.* does: a
// sessions-account control session publishes module.so100.command; the relay
// forwards it on the rover's own-account connection; the leaf carries it to a
// leaf-backed subscriber (module backend stand-in).
func TestRelay_ForwardsModuleCommand(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := natshub.Start(context.Background(), natshub.Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1, Logs: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-module-relay"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, nil) // inherit
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

	// Agent embedded server joined via leaf; module backend stand-in subscribes
	// module.so100.command.
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
	agentNC, err := nats.Connect(agentSrv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer agentNC.Close()
	subj := "waypoint." + roverID + ".module.so100.command"
	got := make(chan string, 1)
	if _, err := agentNC.Subscribe(subj, func(m *nats.Msg) { got <- string(m.Data) }); err != nil {
		t.Fatal(err)
	}
	if err := agentNC.Flush(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let sub interest propagate up the leaf

	// Relay: sessions-account internal connection -> rover-account pool.
	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}
	relayNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(internalJWT, string(internalSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer relayNC.Close()
	pool := roverconn.NewPool(h.Server.ClientURL(), func(id string) (string, string, error) {
		j, s, err := op.MintRoverInternalUserJWT(roverAcctPK, id)
		return j, string(s), err
	})
	defer pool.Close()
	if err := cmdrelay.Start(relayNC, pool); err != nil {
		t.Fatalf("relay start: %v", err)
	}
	if err := relayNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Publisher: sessions-account CONTROL session.
	sessUserJWT, sessUserSeed, err := op.MintSessionUserJWT("u1",
		[]operator.SessionAccess{{RoverID: roverID, Role: "control"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	pubNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(sessUserJWT, string(sessUserSeed)))
	if err != nil {
		t.Fatalf("sessions publisher connect: %v", err)
	}
	defer pubNC.Close()
	if err := pubNC.Publish(subj, []byte("run-calibration")); err != nil {
		t.Fatal(err)
	}
	if err := pubNC.Flush(); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-got:
		if body != "run-calibration" {
			t.Fatalf("payload mismatch: %q", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("module command was not relayed to the rover")
	}
}

// TestRelay_ControlRpcReachesRoverBehindLeaf proves the control RPCs
// (set_mode/estop/recover) relay browser->rover the same way cmd.* does: a
// sessions-account CONTROL session publishes each; the relay forwards it on the
// rover's own-account connection; the leaf carries it to a leaf-backed
// subscriber. Also asserts the republish carries the Waypoint-Relayed header
// (the loop guard the firehose re-import relies on).
func TestRelay_ControlRpcReachesRoverBehindLeaf(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := natshub.Start(context.Background(), natshub.Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1, Logs: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-rpc-relay"
	allowed := []string{"waypoint." + roverID + ".>"}
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, nil) // inherit
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

	// Agent embedded server joined via leaf; core stand-in subscribes rpc.>.
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
	agentNC, err := nats.Connect(agentSrv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer agentNC.Close()
	sub, err := agentNC.SubscribeSync("waypoint." + roverID + ".rpc.>")
	if err != nil {
		t.Fatal(err)
	}
	if err := agentNC.Flush(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let sub interest propagate up the leaf

	// Relay: sessions-account internal connection -> rover-account pool.
	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}
	relayNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(internalJWT, string(internalSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer relayNC.Close()
	pool := roverconn.NewPool(h.Server.ClientURL(), func(id string) (string, string, error) {
		j, s, err := op.MintRoverInternalUserJWT(roverAcctPK, id)
		return j, string(s), err
	})
	defer pool.Close()
	if err := cmdrelay.Start(relayNC, pool); err != nil {
		t.Fatalf("relay start: %v", err)
	}
	if err := relayNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Publisher: sessions-account CONTROL session (proves the control-role grant).
	sessUserJWT, sessUserSeed, err := op.MintSessionUserJWT("u1",
		[]operator.SessionAccess{{RoverID: roverID, Role: "control"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	pubNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(sessUserJWT, string(sessUserSeed)))
	if err != nil {
		t.Fatalf("sessions publisher connect: %v", err)
	}
	defer pubNC.Close()

	for _, leaf := range []string{"set_mode", "estop", "recover"} {
		subj := "waypoint." + roverID + ".rpc." + leaf
		if err := pubNC.Publish(subj, []byte(leaf)); err != nil {
			t.Fatalf("publish %s: %v", subj, err)
		}
	}
	if err := pubNC.Flush(); err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for range []int{0, 1, 2} {
		msg, err := sub.NextMsg(3 * time.Second)
		if err != nil {
			t.Fatalf("control rpc did not reach leaf-backed subscriber via relay: %v (got so far: %v)", err, got)
		}
		got[msg.Subject] = true
		if msg.Header.Get("Waypoint-Relayed") == "" {
			t.Fatalf("relayed %s missing Waypoint-Relayed header (loop guard)", msg.Subject)
		}
	}
	for _, leaf := range []string{"set_mode", "estop", "recover"} {
		subj := "waypoint." + roverID + ".rpc." + leaf
		if !got[subj] {
			t.Fatalf("control rpc %s never arrived; got %v", subj, got)
		}
	}
}

// TestRelay_NoFeedbackLoop guards the storm regression: the rover account
// exports waypoint.<id>.> and the sessions account imports it verbatim, so a
// frame the relay republishes into the rover account re-appears in the sessions
// account on the same subject — where the relay's own waypoint.*.cmd.> sub would
// pick it up and republish it again, looping unbounded. A single published frame
// must yield a bounded number of deliveries, not a flood.
func TestRelay_NoFeedbackLoop(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := natshub.Start(context.Background(), natshub.Config{Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "sim-loop"
	roverAcctJWT, roverAcctPK, _, err := op.MintAccountJWT(roverID, []string{"waypoint." + roverID + ".>"})
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

	subj := "waypoint." + roverID + ".cmd.drive"

	// Relay: sessions-account internal connection -> rover-account pool.
	internalJWT, internalSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}
	relayNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(internalJWT, string(internalSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer relayNC.Close()
	pool := roverconn.NewPool(h.Server.ClientURL(), func(id string) (string, string, error) {
		j, s, err := op.MintRoverInternalUserJWT(roverAcctPK, id)
		return j, string(s), err
	})
	defer pool.Close()
	if err := cmdrelay.Start(relayNC, pool); err != nil {
		t.Fatalf("relay start: %v", err)
	}
	if err := relayNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Sessions-account firehose subscriber counts cmd.drive deliveries — this is
	// what the dashboard firehose pane and the relay's own sub observe.
	fhJWT, fhSeed, err := op.MintInternalUserJWT(0)
	if err != nil {
		t.Fatal(err)
	}
	fhNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(fhJWT, string(fhSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer fhNC.Close()
	var count int64
	if _, err := fhNC.Subscribe(subj, func(*nats.Msg) { atomic.AddInt64(&count, 1) }); err != nil {
		t.Fatal(err)
	}
	if err := fhNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Publish exactly one browser-originated frame.
	sessUserJWT, sessUserSeed, err := op.MintSessionUserJWT("u1",
		[]operator.SessionAccess{{RoverID: roverID, Role: "control"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	pubNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(sessUserJWT, string(sessUserSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer pubNC.Close()
	if err := pubNC.Publish(subj, []byte("drive")); err != nil {
		t.Fatal(err)
	}
	if err := pubNC.Flush(); err != nil {
		t.Fatal(err)
	}

	// Let any loop run, then confirm the count is bounded and has settled.
	time.Sleep(400 * time.Millisecond)
	settled := atomic.LoadInt64(&count)
	time.Sleep(200 * time.Millisecond)
	final := atomic.LoadInt64(&count)
	if final != settled {
		t.Fatalf("cmd.drive count still growing (feedback loop): %d then %d", settled, final)
	}
	// One browser publish + one relayed copy re-imported = 2. Anything beyond
	// a small constant means the relay is reprocessing its own output.
	if final > 3 {
		t.Fatalf("one frame produced %d deliveries — relay feedback loop", final)
	}
}
