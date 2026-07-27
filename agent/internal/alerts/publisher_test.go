package alerts

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	natssrv "github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

const testRoverID = "sim-01"

// startEmbeddedNATS boots an in-process nats-server and returns a connected
// client + cleanup. Mirrors the helper in agent/internal/audit so behavior
// stays consistent across publisher tests.
func startEmbeddedNATS(t *testing.T) (*nats.Conn, func()) {
	t.Helper()
	srv, err := natssrv.StartEmbedded(natssrv.Options{Port: -1})
	if err != nil {
		t.Fatalf("start embedded: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		srv.Shutdown()
		t.Fatalf("wait ready: %v", err)
	}
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		t.Fatalf("nats connect: %v", err)
	}
	return nc, func() {
		nc.Close()
		srv.Shutdown()
	}
}

func TestDeterministicID(t *testing.T) {
	t.Parallel()
	got := DeterministicID("sim-01", "agent.proxy", "proxy_disconnected")
	want := "sim-01:agent.proxy:proxy_disconnected"
	if got != want {
		t.Errorf("DeterministicID = %q, want %q", got, want)
	}
	// Same inputs → same output (used as the dedup key).
	if DeterministicID("sim-01", "agent.proxy", "proxy_disconnected") != want {
		t.Errorf("DeterministicID is not deterministic")
	}
}

func TestPublishRaised_RoundTrip(t *testing.T) {
	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	sub, err := nc.SubscribeSync("waypoint." + testRoverID + ".alert.raised")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	id := DeterministicID(testRoverID, "agent.image", "image_apply_failed")
	md := map[string]string{"url": "https://example.com/img.swu"}
	start := time.Now()
	PublishRaised(nc, testRoverID, id,
		waypointv1.Severity_SEVERITY_FAULT,
		"agent.image", "image_apply_failed",
		"download: status 500",
		md,
	)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("nextmsg: %v", err)
	}
	var ev waypointv1.AlertRaised
	if err := proto.Unmarshal(msg.Data, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Id != id {
		t.Errorf("Id = %q, want %q", ev.Id, id)
	}
	if ev.Source != "agent.image" {
		t.Errorf("Source = %q", ev.Source)
	}
	if ev.Severity != waypointv1.Severity_SEVERITY_FAULT {
		t.Errorf("Severity = %v, want FAULT", ev.Severity)
	}
	if ev.Code != "image_apply_failed" {
		t.Errorf("Code = %q", ev.Code)
	}
	if ev.Message != "download: status 500" {
		t.Errorf("Message = %q", ev.Message)
	}
	if got := ev.Metadata["url"]; got != "https://example.com/img.swu" {
		t.Errorf("Metadata[url] = %q", got)
	}
	if ev.T == nil {
		t.Fatal("nil T")
	}
	at := ev.T.AsTime()
	if at.Before(start.Add(-time.Second)) || at.After(time.Now().Add(time.Second)) {
		t.Errorf("T = %v outside expected window", at)
	}
}

func TestPublishResolved_RoundTrip(t *testing.T) {
	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	sub, err := nc.SubscribeSync("waypoint." + testRoverID + ".alert.resolved")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	id := DeterministicID(testRoverID, "agent.proxy", "proxy_disconnected")
	PublishResolved(nc, testRoverID, id, "leaf reconnected", true)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("nextmsg: %v", err)
	}
	var ev waypointv1.AlertResolved
	if err := proto.Unmarshal(msg.Data, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Id != id {
		t.Errorf("Id = %q, want %q", ev.Id, id)
	}
	if ev.Reason != "leaf reconnected" {
		t.Errorf("Reason = %q", ev.Reason)
	}
	if !ev.AutoResolved {
		t.Errorf("AutoResolved = false, want true")
	}
	if ev.T == nil {
		t.Errorf("nil T")
	}
}

// TestPublish_SkipsOnMissingIDOrRover proves the publisher silently no-ops
// when wiring is incomplete, so half-configured agents don't spam the bus
// with malformed alerts the store can't key on.
func TestPublish_SkipsOnMissingIDOrRover(t *testing.T) {
	nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	sub, err := nc.SubscribeSync("waypoint.>")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	PublishRaised(nc, "", "id", waypointv1.Severity_SEVERITY_FAULT, "s", "c", "m", nil)
	PublishRaised(nc, "rover", "", waypointv1.Severity_SEVERITY_FAULT, "s", "c", "m", nil)
	PublishResolved(nc, "", "id", "r", true)
	PublishResolved(nc, "rover", "", "r", true)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if _, err := sub.NextMsg(100 * time.Millisecond); err == nil {
		t.Errorf("expected no messages on missing id/rover, got one")
	} else if err != nats.ErrTimeout {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPublish_NilConnIsSafe documents that the helpers tolerate a nil nats
// connection — main.go may invoke them before imgNC is set up if the order
// is wrong, and we'd rather no-op than panic.
func TestPublish_NilConnIsSafe(t *testing.T) {
	t.Parallel()
	PublishRaised(nil, "rover", "id", waypointv1.Severity_SEVERITY_FAULT, "s", "c", "m", nil)
	PublishResolved(nil, "rover", "id", "r", true)
}
