package heartbeat

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// embeddedNATS starts an in-process NATS server, returns the client connection
// and registers cleanup. Same pattern as agent/internal/sysinfo/publisher_test.go.
func embeddedNATS(t *testing.T) (*nats.Conn, string) {
	t.Helper()
	s, err := server.NewServer(&server.Options{Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	t.Cleanup(func() { s.Shutdown(); s.WaitForShutdown() })
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc, s.ClientURL()
}

func TestPublisher_emitsAtInterval(t *testing.T) {
	nc, _ := embeddedNATS(t)

	p := New(nc, "rover-dev")
	p.interval = 10 * time.Millisecond

	gotCh := make(chan *waypointv1.Heartbeat, 16)
	sub, err := nc.Subscribe("waypoint.rover-dev.infra.heartbeat", func(m *nats.Msg) {
		var msg waypointv1.Heartbeat
		if err := proto.Unmarshal(m.Data, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		select {
		case gotCh <- &msg:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	time.Sleep(45 * time.Millisecond)
	cancel()

	got := len(gotCh)
	if got < 3 {
		t.Fatalf("expected >=3 messages in 45ms at 10ms interval; got %d", got)
	}

	first := <-gotCh
	if first.GetT() == nil {
		t.Error("t (timestamp) is nil; want set")
	}
	diff := time.Since(first.GetT().AsTime()).Abs()
	if diff > 5*time.Second {
		t.Errorf("first message timestamp far from now: diff=%v", diff)
	}
}

func TestPublisher_emitsOnImmediateStart(t *testing.T) {
	nc, _ := embeddedNATS(t)

	p := New(nc, "rover-dev")
	// Use a very long interval so only the immediate publish fires within 20ms.
	p.interval = 10 * time.Second

	gotCh := make(chan *waypointv1.Heartbeat, 1)
	_, err := nc.Subscribe("waypoint.rover-dev.infra.heartbeat", func(m *nats.Msg) {
		var msg waypointv1.Heartbeat
		if err := proto.Unmarshal(m.Data, &msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		select {
		case gotCh <- &msg:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)

	select {
	case msg := <-gotCh:
		if msg.GetT() == nil {
			t.Error("immediate message has nil timestamp")
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("first heartbeat did not arrive within 20ms; publish-on-start not working")
	}
}

func TestPublisher_ctxCancelStops(t *testing.T) {
	nc, _ := embeddedNATS(t)
	p := New(nc, "rover-dev")
	p.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	time.Sleep(15 * time.Millisecond) // let at least one tick happen
	cancel()

	select {
	case <-done:
		// Run returned within budget; success.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of ctx cancel")
	}
}
