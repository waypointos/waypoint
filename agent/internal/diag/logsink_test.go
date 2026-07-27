package diag

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

type fakeBus struct {
	mu       sync.Mutex
	last     *waypointv1.LogRecord
	subject  string
	fail     bool
	failCalls atomic.Int32
}

func (b *fakeBus) Publish(subject string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail {
		b.failCalls.Add(1)
		return errors.New("bus down")
	}
	rec := &waypointv1.LogRecord{}
	if err := proto.Unmarshal(data, rec); err != nil {
		return err
	}
	b.last = rec
	b.subject = subject
	return nil
}

func TestBusSinkPublishesRecord(t *testing.T) {
	bus := &fakeBus{}
	sink := NewBusSink(bus, "waypoint.rover-x.diag.log", slog.LevelInfo)
	logger := slog.New(sink)
	logger.Info("hello world", "k", "v", "n", 42)

	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		bus.mu.Lock()
		got := bus.last
		bus.mu.Unlock()
		if got != nil {
			if got.Msg != "hello world" {
				t.Fatalf("msg: got %q want %q", got.Msg, "hello world")
			}
			if got.Level != "info" {
				t.Fatalf("level: got %q want info", got.Level)
			}
			if got.Fields["k"] != "v" || got.Fields["n"] != "42" {
				t.Fatalf("fields: got %+v", got.Fields)
			}
			if bus.subject != "waypoint.rover-x.diag.log" {
				t.Fatalf("subject: got %q", bus.subject)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no record published")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBusSinkNonBlockingOnError(t *testing.T) {
	bus := &fakeBus{fail: true}
	sink := NewBusSink(bus, "x", slog.LevelInfo)
	logger := slog.New(sink)
	for i := 0; i < 100; i++ {
		logger.Info("spam")
	}
	if sink.Dropped() < 1 {
		t.Fatalf("expected dropped counter > 0, got %d", sink.Dropped())
	}
	_ = context.Background()
}
