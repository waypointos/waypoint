package audit

import (
	"context"
	"encoding/json"
	"log"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// Start subscribes to waypoint.*.infra.audit for every rover and persists each
// AuditEvent to Postgres via the provided AuditRepo. The subscription is
// automatically torn down when ctx is canceled.
func Start(ctx context.Context, nc *nats.Conn, repo *db.AuditRepo) error {
	sub, err := nc.Subscribe("waypoint.*.infra.audit", func(msg *nats.Msg) {
		var ev waypointv1.AuditEvent
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			log.Printf("audit: bad proto on %s: %v", msg.Subject, err)
			return
		}
		id, err := uuid.Parse(ev.Id)
		if err != nil {
			log.Printf("audit: bad uuid %q: %v", ev.Id, err)
			return
		}
		kind, payload := kindAndPayload(&ev)
		row := db.AuditRow{
			ID:         id,
			OccurredAt: ev.At.AsTime(),
			Source:     ev.Source,
			ActorEmail: ev.ActorEmail,
			RoverID:    ev.RoverId,
			Kind:       kind,
			Payload:    payload,
		}
		if ev.ActorUserId != "" {
			if uid, err := uuid.Parse(ev.ActorUserId); err == nil {
				row.ActorUserID = &uid
			} else {
				log.Printf("audit: bad actor uuid %q: %v", ev.ActorUserId, err)
			}
		}
		if err := repo.Insert(ctx, row); err != nil {
			log.Printf("audit: insert: %v", err)
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

// kindAndPayload turns the oneof body into a (kind, json-payload) pair suitable
// for the audit_events row. The JSON shape mirrors the protobuf fields so the
// dashboard can render rows generically.
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
	case *waypointv1.AuditEvent_RoverDeleted:
		raw, _ := json.Marshal(map[string]any{
			"rover_name":     b.RoverDeleted.RoverName,
			"account_pubkey": b.RoverDeleted.AccountPubkey,
		})
		return "rover_deleted", raw
	}
	return "unknown", json.RawMessage(`{}`)
}
