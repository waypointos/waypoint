package cmdrelay_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/proxy/internal/cmdrelay"
	"github.com/waypointos/waypoint/proxy/internal/natshub"
	"github.com/waypointos/waypoint/proxy/internal/operator"
	"github.com/waypointos/waypoint/proxy/internal/roverconn"
)

// A component module takes commands on module.<id>.<class>.cmd, which the flat
// module.*.command pattern does not match. This covers the wiring the unit
// tests cannot: that infra.modules actually reaches the relay's sessions-account
// connection, that the component leaf is forwarded to the rover account, and
// that the agent's raw servo broker on the same wildcard is not.
func TestRelay_ComponentCmdRelayedAndServoBrokerBlocked(t *testing.T) {
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

	roverID := "sim-component-cmd"
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

	// Rover-account client: stands in for the agent. Publishes the snapshot the
	// relay learns classes from, and subscribes where the module would.
	roverUserJWT, roverUserSeed, err := op.MintRoverUserJWT(roverAcctJWT, nil)
	if err != nil {
		t.Fatal(err)
	}
	roverNC, err := nats.Connect(h.Server.ClientURL(), nats.UserJWTAndSeed(roverUserJWT, string(roverUserSeed)))
	if err != nil {
		t.Fatal(err)
	}
	defer roverNC.Close()
	// One subscription spanning both leaves, so ordering proves the drop: if the
	// servo frame were relayed it would arrive first.
	sub, err := roverNC.SubscribeSync("waypoint." + roverID + ".module.drill.*.cmd")
	if err != nil {
		t.Fatal(err)
	}
	if err := roverNC.Flush(); err != nil {
		t.Fatal(err)
	}

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

	// The agent reports drill as serving component class "drill".
	snap, err := proto.Marshal(&waypointv1.ModuleSnapshot{Modules: []*waypointv1.ModuleInfo{
		{Id: "drill", Component: &waypointv1.ModuleComponent{Class: "drill"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := roverNC.Publish("waypoint."+roverID+".infra.modules", snap); err != nil {
		t.Fatal(err)
	}
	if err := roverNC.Flush(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let the relay learn the class

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
	if err := pubNC.Publish("waypoint."+roverID+".module.drill.servo.cmd", []byte("servo")); err != nil {
		t.Fatal(err)
	}
	if err := pubNC.Publish("waypoint."+roverID+".module.drill.drill.cmd", []byte("jog")); err != nil {
		t.Fatal(err)
	}
	if err := pubNC.Flush(); err != nil {
		t.Fatal(err)
	}

	msg, err := sub.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("component cmd did not reach the rover account via relay: %v", err)
	}
	if got := msg.Subject; got != "waypoint."+roverID+".module.drill.drill.cmd" {
		t.Fatalf("servo broker frame was relayed: %s", got)
	}
	if string(msg.Data) != "jog" {
		t.Fatalf("payload mismatch: %q", msg.Data)
	}
	if extra, err := sub.NextMsg(300 * time.Millisecond); err == nil {
		t.Fatalf("unexpected second frame relayed: %s", extra.Subject)
	}
}
