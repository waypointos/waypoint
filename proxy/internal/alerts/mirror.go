// Package alerts contains the proxy-side mirror subscriber that persists every
// rover's AlertRaised/Acknowledged/Resolved event to Postgres. The rover keeps
// its own SQLite copy (see agent/internal/alerts/store.go); the proxy is the
// authoritative store for cross-rover queries and dashboard rendering.
package alerts

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// Severity-string constants matching the Postgres alert_severity enum.
const (
	severityInfo     = "info"
	severityWarning  = "warning"
	severityCritical = "critical"
)

// StartMirror subscribes to waypoint.*.alert.{raised,acknowledged,resolved}
// for every rover and persists each event via the repo. Subscriptions are
// torn down when ctx is canceled.
//
// FK note: the alerts table references rovers(id); raises for unknown rovers
// fail at the DB level. Such events are logged and dropped — that's the
// expected behavior on a transient race where the rover is publishing before
// it's been registered on the proxy.
//
// Acks with empty user_id are logged and dropped (acks must come from a real
// user). Auto-resolves with empty user_id are accepted (that's the leaf
// auto-resolve path).
func StartMirror(ctx context.Context, nc *nats.Conn, repo *db.AlertsRepo) error {
	raised, err := nc.Subscribe("waypoint.*.alert.raised", func(msg *nats.Msg) {
		roverID := extractRoverID(msg.Subject)
		if roverID == "" {
			return
		}
		var ev waypointv1.AlertRaised
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			log.Printf("alerts/mirror: bad AlertRaised proto on %s: %v", msg.Subject, err)
			return
		}
		row := db.AlertRow{
			ID:       ev.Id,
			RoverID:  roverID,
			Severity: severityToString(ev.Severity),
			Source:   ev.Source,
			Code:     ev.Code,
			Message:  ev.Message,
			Metadata: marshalMetadata(ev.Metadata),
			RaisedAt: timestampOrNow(ev.T),
		}
		if err := repo.Raise(ctx, row); err != nil {
			log.Printf("alerts/mirror: raise %s rover=%s: %v", ev.Id, roverID, err)
		}
	})
	if err != nil {
		return err
	}

	acked, err := nc.Subscribe("waypoint.*.alert.acknowledged", func(msg *nats.Msg) {
		roverID := extractRoverID(msg.Subject)
		if roverID == "" {
			return
		}
		var ev waypointv1.AlertAcknowledged
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			log.Printf("alerts/mirror: bad AlertAcknowledged proto on %s: %v", msg.Subject, err)
			return
		}
		uid, err := uuid.Parse(ev.UserId)
		if err != nil {
			// Acks without a real user are dropped: every ack must come from a
			// human; an empty/invalid UUID is either a bug or a malicious
			// publish, neither of which we want to persist.
			log.Printf("alerts/mirror: drop ack %s: invalid user id %q: %v", ev.Id, ev.UserId, err)
			return
		}
		if err := repo.Acknowledge(ctx, ev.Id, uid, timestampOrNow(ev.T)); err != nil {
			log.Printf("alerts/mirror: ack %s rover=%s: %v", ev.Id, roverID, err)
		}
	})
	if err != nil {
		_ = raised.Unsubscribe()
		return err
	}

	resolved, err := nc.Subscribe("waypoint.*.alert.resolved", func(msg *nats.Msg) {
		roverID := extractRoverID(msg.Subject)
		if roverID == "" {
			return
		}
		var ev waypointv1.AlertResolved
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			log.Printf("alerts/mirror: bad AlertResolved proto on %s: %v", msg.Subject, err)
			return
		}
		var userID *uuid.UUID
		if !ev.AutoResolved {
			parsed, ok := parseUserFromReason(ev.Reason)
			if !ok {
				log.Printf("alerts/mirror: drop manual resolve %s: invalid reason %q (expected 'user:<uuid>')", ev.Id, ev.Reason)
				return
			}
			userID = &parsed
		}
		if err := repo.Resolve(ctx, ev.Id, userID, timestampOrNow(ev.T), ev.AutoResolved); err != nil {
			log.Printf("alerts/mirror: resolve %s rover=%s: %v", ev.Id, roverID, err)
		}
	})
	if err != nil {
		_ = raised.Unsubscribe()
		_ = acked.Unsubscribe()
		return err
	}

	go func() {
		<-ctx.Done()
		_ = raised.Unsubscribe()
		_ = acked.Unsubscribe()
		_ = resolved.Unsubscribe()
	}()
	return nil
}

// timestampOrNow returns t.AsTime() when t is non-nil, otherwise UTC now.
func timestampOrNow(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Now().UTC()
	}
	return t.AsTime()
}

// severityToString maps the protobuf Severity enum to the alert_severity
// Postgres enum string. Defensive default for UNSPECIFIED is "info".
//
// Duplicated from the agent's alerts package on purpose: the proxy and agent
// are independent Go modules and can't import each other's internal packages.
// Both copies are covered by tests in their respective packages.
func severityToString(v waypointv1.Severity) string {
	switch v {
	case waypointv1.Severity_SEVERITY_INFO:
		return severityInfo
	case waypointv1.Severity_SEVERITY_WARN:
		return severityWarning
	case waypointv1.Severity_SEVERITY_FAULT:
		return severityCritical
	default:
		return severityInfo
	}
}

// marshalMetadata turns a proto map<string,string> into deterministic JSON
// bytes for storage. Returns nil for empty/nil maps so the column stores NULL
// rather than `{}`.
func marshalMetadata(m map[string]string) json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return raw
}

// extractRoverID parses "waypoint.<id>.alert.X" → <id>. Returns "" for
// malformed subjects (which become silent drops).
//
// Duplicated from proxy/internal/audit (extractRoverID is package-private
// there). Same module-isolation reasoning as above: we'd rather copy 8 lines
// than introduce a shared subjects package mid-phase.
func extractRoverID(subject string) string {
	parts := strings.SplitN(subject, ".", 3)
	if len(parts) < 2 || parts[0] != "waypoint" {
		return ""
	}
	return parts[1]
}

// parseUserFromReason recovers a UUID from a "user:<uuid>" reason string used
// by manual resolves. Returns (uuid, true) on success, (zero, false) otherwise.
func parseUserFromReason(reason string) (uuid.UUID, bool) {
	const prefix = "user:"
	if !strings.HasPrefix(reason, prefix) {
		return uuid.UUID{}, false
	}
	uid, err := uuid.Parse(strings.TrimPrefix(reason, prefix))
	if err != nil {
		return uuid.UUID{}, false
	}
	return uid, true
}
