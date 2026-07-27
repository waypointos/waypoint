package audit

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	natssrv "github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestExtractRoverID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"waypoint.sim-01.cmd.drive", "sim-01"},
		{"waypoint.sim-01.infra.audit", "sim-01"},
		{"notwaypoint.foo.bar", ""},
		{"waypoint", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := extractRoverID(c.in)
		if got != c.want {
			t.Errorf("extractRoverID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPublisher_RoundTrip(t *testing.T) {
	srv, nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()
	_ = srv

	// Subscribe before publishing.
	sub, err := nc.SubscribeSync("waypoint.sim-01.infra.audit")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	actor := Actor{ID: "user-uuid", Email: "user@example.com"}
	p := NewPublisher(nc, actor, "test")

	const roverID = "sim-01"
	const subject = "waypoint.sim-01.cmd.drive"
	const payloadBytes = 42
	const transport = "ws"
	const remoteIP = "192.0.2.1"
	const targetUserID = "target-uuid"
	const action = "grant"
	const role = "control"
	duration := 1234 * time.Millisecond
	const fromVersion = "1.0.0"
	const toVersion = "1.1.0"
	const url = "https://example.com/img"

	start := time.Now()
	p.LogCommand(subject, payloadBytes)
	p.LogAccessChange(targetUserID, action, role, roverID)
	p.LogSessionOpened(roverID, transport, remoteIP)
	p.LogSessionClosed(roverID, transport, duration)
	p.LogImageApplied(roverID, fromVersion, toVersion, url)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after publish: %v", err)
	}

	events := make([]*waypointv1.AuditEvent, 0, 5)
	for i := 0; i < 5; i++ {
		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			t.Fatalf("nextmsg %d: %v", i, err)
		}
		var ev waypointv1.AuditEvent
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			t.Fatalf("unmarshal %d: %v", i, err)
		}
		events = append(events, &ev)
	}

	for i, ev := range events {
		if ev.Id == "" {
			t.Errorf("event %d: empty Id", i)
		}
		if ev.At == nil {
			t.Fatalf("event %d: nil At", i)
		}
		at := ev.At.AsTime()
		if at.Before(start.Add(-time.Second)) || at.After(time.Now().Add(time.Second)) {
			t.Errorf("event %d: At=%v outside expected window", i, at)
		}
		if ev.Source != "test" {
			t.Errorf("event %d: Source=%q, want %q", i, ev.Source, "test")
		}
		if ev.ActorUserId != "user-uuid" {
			t.Errorf("event %d: ActorUserId=%q", i, ev.ActorUserId)
		}
		if ev.ActorEmail != "user@example.com" {
			t.Errorf("event %d: ActorEmail=%q", i, ev.ActorEmail)
		}
		if ev.RoverId != roverID {
			t.Errorf("event %d: RoverId=%q, want %q", i, ev.RoverId, roverID)
		}
	}

	// Body assertions, in publish order.
	switch body := events[0].Body.(type) {
	case *waypointv1.AuditEvent_Command:
		if body.Command.Subject != subject {
			t.Errorf("LogCommand Subject=%q", body.Command.Subject)
		}
		if body.Command.PayloadBytes != int32(payloadBytes) {
			t.Errorf("LogCommand PayloadBytes=%d", body.Command.PayloadBytes)
		}
	default:
		t.Errorf("events[0] body type=%T, want *AuditEvent_Command", body)
	}

	switch body := events[1].Body.(type) {
	case *waypointv1.AuditEvent_Access:
		if body.Access.TargetUserId != targetUserID {
			t.Errorf("LogAccessChange TargetUserId=%q", body.Access.TargetUserId)
		}
		if body.Access.Action != action {
			t.Errorf("LogAccessChange Action=%q", body.Access.Action)
		}
		if body.Access.Role != role {
			t.Errorf("LogAccessChange Role=%q", body.Access.Role)
		}
	default:
		t.Errorf("events[1] body type=%T, want *AuditEvent_Access", body)
	}

	switch body := events[2].Body.(type) {
	case *waypointv1.AuditEvent_SessionOpen:
		if body.SessionOpen.Transport != transport {
			t.Errorf("LogSessionOpened Transport=%q", body.SessionOpen.Transport)
		}
		if body.SessionOpen.RemoteIp != remoteIP {
			t.Errorf("LogSessionOpened RemoteIp=%q", body.SessionOpen.RemoteIp)
		}
	default:
		t.Errorf("events[2] body type=%T, want *AuditEvent_SessionOpen", body)
	}

	switch body := events[3].Body.(type) {
	case *waypointv1.AuditEvent_SessionClose:
		if body.SessionClose.Transport != transport {
			t.Errorf("LogSessionClosed Transport=%q", body.SessionClose.Transport)
		}
		if body.SessionClose.DurationMs != duration.Milliseconds() {
			t.Errorf("LogSessionClosed DurationMs=%d, want %d",
				body.SessionClose.DurationMs, duration.Milliseconds())
		}
	default:
		t.Errorf("events[3] body type=%T, want *AuditEvent_SessionClose", body)
	}

	switch body := events[4].Body.(type) {
	case *waypointv1.AuditEvent_Image:
		if body.Image.FromVersion != fromVersion {
			t.Errorf("LogImageApplied FromVersion=%q", body.Image.FromVersion)
		}
		if body.Image.ToVersion != toVersion {
			t.Errorf("LogImageApplied ToVersion=%q", body.Image.ToVersion)
		}
		if body.Image.Url != url {
			t.Errorf("LogImageApplied Url=%q", body.Image.Url)
		}
	default:
		t.Errorf("events[4] body type=%T, want *AuditEvent_Image", body)
	}
}

func TestPublisher_LogCommand_SkipsWhenSubjectUnparseable(t *testing.T) {
	_, nc, cleanup := startEmbeddedNATS(t)
	defer cleanup()

	// Wildcard subscribe so any publish would be observed.
	sub, err := nc.SubscribeSync("waypoint.>")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	p := NewPublisher(nc, Actor{ID: "u", Email: "u@example.com"}, "test")

	// Subjects that don't yield a rover id must be no-ops, no panic.
	p.LogCommand("notwaypoint.sim-01.cmd.drive", 7)
	p.LogCommand("", 7)
	p.LogCommand("waypoint", 7)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush after no-op publishes: %v", err)
	}

	// Sanity-check: nothing arrived.
	if _, err := sub.NextMsg(100 * time.Millisecond); err == nil {
		t.Errorf("expected no message, got one")
	} else if err != nats.ErrTimeout {
		t.Errorf("unexpected error type: %v", err)
	}
}

func startEmbeddedNATS(t *testing.T) (*natssrv.Server, *nats.Conn, func()) {
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
	cleanup := func() {
		nc.Close()
		srv.Shutdown()
	}
	return srv, nc, cleanup
}
