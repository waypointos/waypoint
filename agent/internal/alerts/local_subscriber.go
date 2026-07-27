package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// StartLocalSubscriber subscribes to the rover's own alert subject tree and
// applies each event to the local SQLite store:
//
//	waypoint.<roverID>.alert.raised        → Store.Raise
//	waypoint.<roverID>.alert.acknowledged  → Store.Acknowledge
//	waypoint.<roverID>.alert.resolved      → Store.Resolve
//
// The subscriptions are torn down when ctx is canceled. Mirrors the pattern in
// audit.StartLocalSubscriber.
//
// Acknowledge/Resolve events for unknown ids are logged and dropped — the
// store rejects them with sql.ErrNoRows. This is the right behavior on a
// freshly-rebooted rover that missed the original raise: the proxy is the
// authoritative copy, and the rover only needs to render what it has.
func StartLocalSubscriber(ctx context.Context, nc *nats.Conn, roverID string, store *Store) error {
	prefix := "waypoint." + roverID + ".alert."

	raised, err := nc.Subscribe(prefix+"raised", func(msg *nats.Msg) {
		var ev waypointv1.AlertRaised
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			slog.Warn(fmt.Sprintf("alerts/local: bad AlertRaised proto on %s: %v", msg.Subject, err))
			return
		}
		a := StoredAlert{
			ID:       ev.Id,
			RoverID:  roverID,
			Severity: SeverityToString(int32(ev.Severity)),
			Source:   ev.Source,
			Code:     ev.Code,
			Message:  ev.Message,
			Metadata: marshalMetadata(ev.Metadata),
			RaisedAt: timestampOrNow(ev.T),
		}
		if err := store.Raise(a); err != nil {
			slog.Warn(fmt.Sprintf("alerts/local: raise %s: %v", ev.Id, err))
		}
	})
	if err != nil {
		return err
	}

	acked, err := nc.Subscribe(prefix+"acknowledged", func(msg *nats.Msg) {
		var ev waypointv1.AlertAcknowledged
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			slog.Warn(fmt.Sprintf("alerts/local: bad AlertAcknowledged proto on %s: %v", msg.Subject, err))
			return
		}
		if err := store.Acknowledge(ev.Id, ev.UserId, timestampOrNow(ev.T)); err != nil {
			slog.Warn(fmt.Sprintf("alerts/local: ack %s: %v", ev.Id, err))
		}
	})
	if err != nil {
		_ = raised.Unsubscribe()
		return err
	}

	resolved, err := nc.Subscribe(prefix+"resolved", func(msg *nats.Msg) {
		var ev waypointv1.AlertResolved
		if err := proto.Unmarshal(msg.Data, &ev); err != nil {
			slog.Warn(fmt.Sprintf("alerts/local: bad AlertResolved proto on %s: %v", msg.Subject, err))
			return
		}
		userID := ""
		if !ev.AutoResolved {
			userID = userIDFromReason(ev.Reason)
		}
		if err := store.Resolve(ev.Id, userID, timestampOrNow(ev.T), ev.AutoResolved); err != nil {
			slog.Warn(fmt.Sprintf("alerts/local: resolve %s: %v", ev.Id, err))
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
// A nil proto timestamp shouldn't happen on the wire (the publisher always
// sets it), but we'd rather have a usable raised_at than a zero one.
func timestampOrNow(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Now().UTC()
	}
	return t.AsTime()
}

// marshalMetadata turns a proto map<string,string> into deterministic JSON
// bytes for storage. Returns nil for empty/nil maps so the column stores NULL
// rather than `{}`.
func marshalMetadata(m map[string]string) json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	// json.Marshal sorts map keys alphabetically, which keeps the storage
	// representation stable across runs — handy for diff-debugging.
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return raw
}

// userIDFromReason extracts the user id from a resolved-by-user reason string.
// The publisher convention (see protocol/messages/alerts.proto) is
// "user:<user-id>" for human-triggered resolves; anything else is treated as
// "no user id available" — the store keeps it as an empty string.
//
// We deliberately keep this side of the protocol forgiving: the proxy's
// AckResolve endpoint publishes the structured reason, but other publishers
// and tests emit free-form reasons.
func userIDFromReason(reason string) string {
	const prefix = "user:"
	if strings.HasPrefix(reason, prefix) {
		return strings.TrimPrefix(reason, prefix)
	}
	return ""
}
