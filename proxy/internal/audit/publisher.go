// Package audit emits AuditEvent protobufs onto NATS so privileged actions
// (commands, access changes, sessions, image rollouts) are durably recorded.
// The proxy persists these to Postgres; the agent persists to SQLite.
//
// This package is intentionally duplicated between the agent and proxy modules
// — the two modules are independent Go modules and can't import each other's
// internal packages (same pattern as internal/magiclink).
package audit

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// Actor identifies the privileged caller behind an event. Empty ID means a
// system-issued event (e.g. agent emitting ImageApplied on its own).
type Actor struct {
	ID    string // UUID string; empty for system-issued events
	Email string
}

// Publisher emits AuditEvent messages on waypoint.<rover-id>.infra.audit.
// Construct one per logical actor/source pair (e.g. a single WS session).
type Publisher struct {
	nc     *nats.Conn
	actor  Actor
	source string // "proxy-ws" / "agent-ws" / "proxy-api" / "agent-rpc"
}

// NewPublisher binds an Actor + source to a NATS connection.
func NewPublisher(nc *nats.Conn, actor Actor, source string) *Publisher {
	return &Publisher{nc: nc, actor: actor, source: source}
}

// LogCommand emits a CommandIssued event. subject is the NATS subject the
// privileged client published on; payloadBytes is the byte length of the
// command payload (no payload contents are recorded — we don't audit user
// data). If the subject doesn't yield a rover id, the call is a silent no-op.
func (p *Publisher) LogCommand(subject string, payloadBytes int) {
	roverID := extractRoverID(subject)
	ev := p.base(roverID)
	ev.Body = &waypointv1.AuditEvent_Command{Command: &waypointv1.CommandIssued{
		Subject:      subject,
		PayloadBytes: int32(payloadBytes),
	}}
	p.publish(roverID, ev)
}

// LogAccessChange emits an AccessChanged event (grant or revoke).
func (p *Publisher) LogAccessChange(targetUserID, action, role, roverID string) {
	ev := p.base(roverID)
	ev.Body = &waypointv1.AuditEvent_Access{Access: &waypointv1.AccessChanged{
		TargetUserId: targetUserID,
		Action:       action,
		Role:         role,
	}}
	p.publish(roverID, ev)
}

// LogSessionOpened emits a SessionOpened event.
func (p *Publisher) LogSessionOpened(roverID, transport, remoteIP string) {
	ev := p.base(roverID)
	ev.Body = &waypointv1.AuditEvent_SessionOpen{SessionOpen: &waypointv1.SessionOpened{
		Transport: transport,
		RemoteIp:  remoteIP,
	}}
	p.publish(roverID, ev)
}

// LogSessionClosed emits a SessionClosed event with the wall-clock duration.
func (p *Publisher) LogSessionClosed(roverID, transport string, duration time.Duration) {
	ev := p.base(roverID)
	ev.Body = &waypointv1.AuditEvent_SessionClose{SessionClose: &waypointv1.SessionClosed{
		Transport:  transport,
		DurationMs: duration.Milliseconds(),
	}}
	p.publish(roverID, ev)
}

// LogRoverDeleted emits a RoverDeleted event. The rover row is about to be
// removed from the database, so this must be published before the delete so
// the audit subscriber can persist the row while the rover_id is still
// meaningful. (audit_events.rover_id is free-text, not a FK, so the row
// survives even after the rover is gone.)
func (p *Publisher) LogRoverDeleted(roverID, name, accountPubkey string) {
	ev := p.base(roverID)
	ev.Body = &waypointv1.AuditEvent_RoverDeleted{RoverDeleted: &waypointv1.RoverDeleted{
		RoverName:     name,
		AccountPubkey: accountPubkey,
	}}
	p.publish(roverID, ev)
}

// LogImageApplied emits an ImageApplied event when the rover boots into a new
// system image (or rolls back).
func (p *Publisher) LogImageApplied(roverID, fromVersion, toVersion, url string) {
	ev := p.base(roverID)
	ev.Body = &waypointv1.AuditEvent_Image{Image: &waypointv1.ImageApplied{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Url:         url,
	}}
	p.publish(roverID, ev)
}

func (p *Publisher) base(roverID string) *waypointv1.AuditEvent {
	return &waypointv1.AuditEvent{
		Id:          uuid.NewString(),
		At:          timestamppb.Now(),
		Source:      p.source,
		ActorUserId: p.actor.ID,
		ActorEmail:  p.actor.Email,
		RoverId:     roverID,
	}
}

func (p *Publisher) publish(roverID string, ev *waypointv1.AuditEvent) {
	if roverID == "" {
		return
	}
	body, err := proto.Marshal(ev)
	if err != nil {
		return
	}
	_ = p.nc.Publish("waypoint."+roverID+".infra.audit", body)
}

// extractRoverID parses "waypoint.<id>.X.Y..." → <id>. Returns "" if the
// subject doesn't follow the expected shape — callers treat the empty string
// as "skip this publish."
func extractRoverID(subject string) string {
	parts := strings.SplitN(subject, ".", 3)
	if len(parts) < 2 || parts[0] != "waypoint" {
		return ""
	}
	return parts[1]
}
