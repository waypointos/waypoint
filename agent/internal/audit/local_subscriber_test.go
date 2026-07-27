package audit

import (
	"context"
	"testing"
	"time"
)

// waitForRowCount polls store.List until it returns at least want rows or the
// deadline expires. Returns the most recent result.
func waitForRowCount(t *testing.T, store *Store, want int, deadline time.Duration) []Row {
	t.Helper()
	var last []Row
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		rows, err := store.List(Filter{})
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

func TestLocalSubscriber_PersistsCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMem(t)

	if err := StartLocalSubscriber(ctx, nc, "sim-01", store); err != nil {
		t.Fatalf("start subscriber: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	p := NewPublisher(nc, Actor{ID: "user-uuid", Email: "alice@example.com"}, "agent-rpc")
	const subject = "waypoint.sim-01.cmd.drive"
	p.LogCommand(subject, 42)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	rows := waitForRowCount(t, store, 1, 2*time.Second)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Kind != "command" {
		t.Errorf("Kind = %q, want %q", r.Kind, "command")
	}
	if r.RoverID != "sim-01" {
		t.Errorf("RoverID = %q", r.RoverID)
	}
	if r.Source != "agent-rpc" {
		t.Errorf("Source = %q", r.Source)
	}
	if r.ActorUserID != "user-uuid" {
		t.Errorf("ActorUserID = %q", r.ActorUserID)
	}
	if r.ActorEmail != "alice@example.com" {
		t.Errorf("ActorEmail = %q", r.ActorEmail)
	}
	if r.ID == "" {
		t.Errorf("ID empty")
	}
	if len(r.Payload) == 0 || string(r.Payload) == "{}" {
		t.Errorf("Payload empty/blank: %s", r.Payload)
	}
}

func TestLocalSubscriber_IgnoresMalformedProto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMem(t)

	if err := StartLocalSubscriber(ctx, nc, "sim-01", store); err != nil {
		t.Fatalf("start subscriber: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Random bytes — not a valid AuditEvent proto.
	if err := nc.Publish("waypoint.sim-01.infra.audit", []byte{0xff, 0xff, 0xff, 0xff, 0xff}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Give the subscriber a chance to (not) insert.
	time.Sleep(200 * time.Millisecond)
	rows, err := store.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestLocalSubscriber_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	_, nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMem(t)

	if err := StartLocalSubscriber(ctx, nc, "sim-01", store); err != nil {
		t.Fatalf("start subscriber: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Cancel; give the cleanup goroutine a moment to unsubscribe.
	cancel()
	time.Sleep(100 * time.Millisecond)

	p := NewPublisher(nc, Actor{ID: "u", Email: "u@example.com"}, "agent-rpc")
	p.LogCommand("waypoint.sim-01.cmd.drive", 7)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	rows, err := store.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows after cancel, want 0", len(rows))
	}
}
