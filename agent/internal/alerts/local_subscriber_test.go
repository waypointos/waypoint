package alerts

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// waitForActiveCount polls store.ListActive until at least want rows are
// present or the deadline expires.
func waitForActiveCount(t *testing.T, s *Store, want int, deadline time.Duration) []StoredAlert {
	t.Helper()
	var last []StoredAlert
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		rows, err := s.ListActive("")
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

func waitForState(t *testing.T, s *Store, id, wantState string, deadline time.Duration) StoredAlert {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		// History list includes resolved rows; active doesn't.
		rows, err := s.GetHistory("", time.Unix(0, 0))
		if err != nil {
			t.Fatalf("history: %v", err)
		}
		for _, r := range rows {
			if r.ID == id && r.State == wantState {
				return r
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("state %q on id %q not observed within %v", wantState, id, deadline)
	return StoredAlert{}
}

func TestLocalSubscriber_RaiseAckResolveFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMemStore(t)

	if err := StartLocalSubscriber(ctx, nc, testRoverID, store); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// 1. Raise via the publisher helper.
	id := DeterministicID(testRoverID, "agent.proxy", "proxy_disconnected")
	PublishRaised(nc, testRoverID, id,
		waypointv1.Severity_SEVERITY_FAULT,
		"agent.proxy", "proxy_disconnected",
		"leaf disconnected from hub",
		map[string]string{"hint": "check uplink"},
	)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after raise: %v", err)
	}

	rows := waitForActiveCount(t, store, 1, 2*time.Second)
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.ID != id {
		t.Errorf("ID = %q, want %q", r.ID, id)
	}
	if r.Severity != SeverityCritical {
		t.Errorf("Severity = %q, want critical", r.Severity)
	}
	if r.State != StateActive {
		t.Errorf("State = %q, want active", r.State)
	}
	if r.Source != "agent.proxy" {
		t.Errorf("Source = %q", r.Source)
	}
	if r.Code != "proxy_disconnected" {
		t.Errorf("Code = %q", r.Code)
	}
	if string(r.Metadata) == "" {
		t.Errorf("Metadata empty, want json")
	}

	// 2. Acknowledge — directly publish the proto (the agent publisher only
	// exposes Raised/Resolved; acks are emitted by the proxy API).
	publishProto(t, nc, "waypoint."+testRoverID+".alert.acknowledged", &waypointv1.AlertAcknowledged{
		T:      timestamppb.Now(),
		Id:     id,
		UserId: "user-uuid",
	})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after ack: %v", err)
	}
	acked := waitForState(t, store, id, StateAcknowledged, 2*time.Second)
	if acked.AcknowledgedBy != "user-uuid" {
		t.Errorf("AcknowledgedBy = %q", acked.AcknowledgedBy)
	}
	if acked.AcknowledgedAt == nil {
		t.Errorf("AcknowledgedAt nil after ack")
	}

	// 3. Resolve via the publisher helper (auto-resolved).
	PublishResolved(nc, testRoverID, id, "leaf reconnected", true)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after resolve: %v", err)
	}
	resolved := waitForState(t, store, id, StateResolved, 2*time.Second)
	if !resolved.AutoResolved {
		t.Errorf("AutoResolved = false, want true")
	}
	if resolved.ResolvedAt == nil {
		t.Errorf("ResolvedAt nil after resolve")
	}

	// 4. Confirm it's gone from the active list.
	active, err := store.ListActive("")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active rows = %d, want 0 after resolve", len(active))
	}
}

func TestLocalSubscriber_IgnoresMalformedProto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMemStore(t)

	if err := StartLocalSubscriber(ctx, nc, testRoverID, store); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	for _, subj := range []string{
		"waypoint." + testRoverID + ".alert.raised",
		"waypoint." + testRoverID + ".alert.acknowledged",
		"waypoint." + testRoverID + ".alert.resolved",
	} {
		if err := nc.Publish(subj, []byte{0xff, 0xff, 0xff}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	rows, err := store.ListActive("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestLocalSubscriber_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMemStore(t)

	if err := StartLocalSubscriber(ctx, nc, testRoverID, store); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	PublishRaised(nc, testRoverID, "id-after-cancel",
		waypointv1.Severity_SEVERITY_WARN, "agent.test", "test", "msg", nil)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	rows, err := store.ListActive("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d after cancel, want 0", len(rows))
	}
}

func TestLocalSubscriber_ResolveByUserExtractsID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	store := openMemStore(t)

	if err := StartLocalSubscriber(ctx, nc, testRoverID, store); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	id := DeterministicID(testRoverID, "agent.test", "by_user")
	PublishRaised(nc, testRoverID, id, waypointv1.Severity_SEVERITY_WARN, "agent.test", "by_user", "msg", nil)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	waitForActiveCount(t, store, 1, 2*time.Second)

	// reason "user:abc-123", auto_resolved=false → ResolvedBy "abc-123"
	PublishResolved(nc, testRoverID, id, "user:abc-123", false)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush resolve: %v", err)
	}

	r := waitForState(t, store, id, StateResolved, 2*time.Second)
	if r.AutoResolved {
		t.Errorf("AutoResolved = true, want false")
	}
	if r.ResolvedBy != "abc-123" {
		t.Errorf("ResolvedBy = %q, want abc-123", r.ResolvedBy)
	}
}

func TestUserIDFromReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"user:abc", "abc"},
		{"user:", ""},
		{"auto", ""},
		{"", ""},
		{"leaf reconnected", ""},
	}
	for _, c := range cases {
		if got := userIDFromReason(c.in); got != c.want {
			t.Errorf("userIDFromReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// publishProto serializes m and publishes it on subj — used in the ack test
// because the agent's publisher.go intentionally only exposes Raised/Resolved.
func publishProto(t *testing.T, nc *nats.Conn, subj string, m proto.Message) {
	t.Helper()
	body, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := nc.Publish(subj, body); err != nil {
		t.Fatalf("publish: %v", err)
	}
}
