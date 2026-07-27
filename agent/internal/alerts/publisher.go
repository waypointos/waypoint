// Package alerts publishes AlertRaised / AlertResolved protobufs on
// waypoint.<roverID>.alert.{raised,resolved}. Used by:
//
//   - the leaf-node watcher (proxy_disconnected raise/auto-resolve)
//   - the image apply RPC (image_apply_failed raise on download/verify
//     failures).
//
// Both helpers are best-effort: a marshal or publish error is logged-and-
// dropped so callers don't have to plumb the error up. The dashboard will
// re-observe state on the next retained snapshot anyway.
package alerts

import (
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// PublishRaised publishes an AlertRaised on waypoint.<roverID>.alert.raised.
//
// id must be stable across the alert's lifecycle so the alert store can
// dedupe repeated raises into a single row — use DeterministicID. An empty
// id or empty roverID is a silent no-op so callers don't crash when wiring
// is half-set-up (e.g. agent boots without identity yet).
func PublishRaised(nc *nats.Conn, roverID, id string, sev waypointv1.Severity, source, code, message string, metadata map[string]string) {
	if id == "" || roverID == "" || nc == nil {
		return
	}
	msg := &waypointv1.AlertRaised{
		T:        timestamppb.Now(),
		Id:       id,
		Source:   source,
		Severity: sev,
		Code:     code,
		Message:  message,
		Metadata: metadata,
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	_ = nc.Publish("waypoint."+roverID+".alert.raised", body)
}

// PublishResolved publishes an AlertResolved on
// waypoint.<roverID>.alert.resolved. autoResolved=true marks programmatic
// resolution (e.g. leaf reconnect, successful first-boot of a new image);
// reason carries a human-readable hint surfaced in the dashboard.
func PublishResolved(nc *nats.Conn, roverID, id, reason string, autoResolved bool) {
	if id == "" || roverID == "" || nc == nil {
		return
	}
	msg := &waypointv1.AlertResolved{
		T:            timestamppb.Now(),
		Id:           id,
		Reason:       reason,
		AutoResolved: autoResolved,
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	_ = nc.Publish("waypoint."+roverID+".alert.resolved", body)
}

// DeterministicID returns a stable identifier for an alert source so repeated
// raises during a single boot are idempotent at the store level. Format:
// <roverID>:<source>:<code>. e.g.
//
//	sim-01:agent.proxy:proxy_disconnected
//	sim-01:agent.image:image_apply_failed
//
// We deliberately don't use a ULID — the core watchdog and this
// package both need to converge on the same id without coordination.
func DeterministicID(roverID, source, code string) string {
	return roverID + ":" + source + ":" + code
}
