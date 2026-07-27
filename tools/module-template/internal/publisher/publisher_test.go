package publisher

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/test"
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	examplev1 "github.com/waypointos/waypoint-module-example/protocol/gen/go"
)

// runServer starts an in-process NATS server for the test.
func runServer(t *testing.T) (*natsgo.Conn, func()) {
	t.Helper()
	srv := natsserver.RunRandClientPortServer()
	nc, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	return nc, func() { nc.Close(); srv.Shutdown() }
}

func TestPublisher_PublishesStats(t *testing.T) {
	nc, stop := runServer(t)
	defer stop()

	sub, err := nc.SubscribeSync("waypoint.rover-1.module.example.stats")
	if err != nil {
		t.Fatal(err)
	}

	pub, err := New(nc, "rover-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pub.Run(ctx)

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("no stats published: %v", err)
	}
	var stats examplev1.ExampleStats
	if err := proto.Unmarshal(msg.Data, &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Count < 1 {
		t.Errorf("count = %d, want >= 1", stats.Count)
	}
	if stats.GetMessage() != "hello" {
		t.Errorf("message = %q, want hello", stats.GetMessage())
	}
}

func TestPublisher_AnswersHealthProbe(t *testing.T) {
	nc, stop := runServer(t)
	defer stop()

	if _, err := New(nc, "rover-1", ""); err != nil {
		t.Fatal(err)
	}
	reply, err := nc.Request("waypoint.rover-1.module.example.health.ready", nil, 2*time.Second)
	if err != nil {
		t.Fatalf("health probe failed: %v", err)
	}
	if string(reply.Data) != "ok" {
		t.Errorf("health reply = %q, want ok", reply.Data)
	}
}
