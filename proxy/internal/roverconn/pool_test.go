package roverconn_test

import (
	"context"
	"testing"
	"time"

	"github.com/waypointos/waypoint/proxy/internal/natshub"
	"github.com/waypointos/waypoint/proxy/internal/operator"
	"github.com/waypointos/waypoint/proxy/internal/roverconn"
)

func TestPool_ConnInRoverAccountCanPubSubAndCaches(t *testing.T) {
	op, err := operator.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := natshub.Start(context.Background(), natshub.Config{
		Operator: op, ClientPort: -1, LeafWSPort: -1, HTTPMonPort: -1, Logs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown()
	if !h.Server.ReadyForConnections(2 * time.Second) {
		t.Fatal("hub not ready")
	}

	roverID := "pool-test"
	allowed := []string{"waypoint." + roverID + ".>"}
	acctJWT, acctPK, _, err := op.MintAccountJWT(roverID, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterAccount(acctJWT); err != nil {
		t.Fatal(err)
	}

	pool := roverconn.NewPool(h.Server.ClientURL(), func(id string) (string, string, error) {
		j, s, err := op.MintRoverInternalUserJWT(acctPK, id)
		return j, string(s), err
	})
	defer pool.Close()

	nc, err := pool.Conn(roverID)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	sub, err := nc.SubscribeSync("waypoint." + roverID + ".rpc.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("waypoint."+roverID+".rpc.test", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("pub/sub in rover account failed: %v", err)
	}
	if string(msg.Data) != "hi" {
		t.Fatalf("payload mismatch: %q", msg.Data)
	}

	// Second call returns the same cached connection.
	nc2, err := pool.Conn(roverID)
	if err != nil {
		t.Fatal(err)
	}
	if nc2 != nc {
		t.Fatal("expected cached connection on second Conn()")
	}

	// Remove closes + evicts; a later Conn() reconnects fresh.
	pool.Remove(roverID)
	if !nc.IsClosed() {
		t.Fatal("Remove did not close the connection")
	}
}
