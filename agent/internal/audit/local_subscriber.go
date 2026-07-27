package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// StartLocalSubscriber subscribes to the rover's own audit subject and persists
// events to the local SQLite store. roverID limits the subscription to this
// rover only (the agent only ever sees its own subject anyway, but explicit is
// better than a wildcard on an on-rover bus).
//
// The subscription is torn down when ctx is canceled.
func StartLocalSubscriber(ctx context.Context, nc *nats.Conn, roverID string, store *Store) error {
	subject := "waypoint." + roverID + ".infra.audit"
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		var ev waypointv1.AuditEvent
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			slog.Warn(fmt.Sprintf("audit/local: bad proto on %s: %v", msg.Subject, err))
			return
		}
		kind, payload := kindAndPayload(&ev)
		row := Row{
			ID:          ev.Id,
			OccurredAt:  ev.At.AsTime(),
			Source:      ev.Source,
			ActorUserID: ev.ActorUserId,
			ActorEmail:  ev.ActorEmail,
			RoverID:     ev.RoverId,
			Kind:        kind,
			Payload:     payload,
		}
		if err := store.Insert(row); err != nil {
			slog.Warn(fmt.Sprintf("audit/local: insert: %v", err))
		}
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()
	return nil
}

// kindAndPayload turns the AuditEvent oneof body into a (kind, json-payload)
// pair suitable for the audit_events row. The JSON shape mirrors the protobuf
// fields so the dashboard can render rows generically.
//
// Yes, this is duplicated from the proxy's subscriber.go — same module-isolation
// reason as the magiclink package. The two modules don't import each other.
func kindAndPayload(ev *waypointv1.AuditEvent) (string, json.RawMessage) {
	switch b := ev.Body.(type) {
	case *waypointv1.AuditEvent_Command:
		raw, _ := json.Marshal(map[string]any{
			"subject":       b.Command.Subject,
			"payload_bytes": b.Command.PayloadBytes,
		})
		return "command", raw
	case *waypointv1.AuditEvent_Access:
		raw, _ := json.Marshal(map[string]any{
			"target_user_id": b.Access.TargetUserId,
			"action":         b.Access.Action,
			"role":           b.Access.Role,
		})
		return "access", raw
	case *waypointv1.AuditEvent_SessionOpen:
		raw, _ := json.Marshal(map[string]any{
			"transport": b.SessionOpen.Transport,
			"remote_ip": b.SessionOpen.RemoteIp,
		})
		return "session_open", raw
	case *waypointv1.AuditEvent_SessionClose:
		raw, _ := json.Marshal(map[string]any{
			"transport":   b.SessionClose.Transport,
			"duration_ms": b.SessionClose.DurationMs,
		})
		return "session_close", raw
	case *waypointv1.AuditEvent_Image:
		raw, _ := json.Marshal(map[string]any{
			"from_version": b.Image.FromVersion,
			"to_version":   b.Image.ToVersion,
			"url":          b.Image.Url,
		})
		return "image", raw
	}
	return "unknown", json.RawMessage(`{}`)
}
