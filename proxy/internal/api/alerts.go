package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// alertsRepo narrows db.AlertsRepo to the methods our handlers call. The
// interface lets unit tests substitute an in-memory fake without standing up
// Postgres for branches that don't need it.
type alertsRepo interface {
	ListActive(ctx context.Context, roverID string) ([]db.AlertRow, error)
	GetByID(ctx context.Context, id string) (db.AlertRow, error)
	Acknowledge(ctx context.Context, id string, userID uuid.UUID, at time.Time) error
	Resolve(ctx context.Context, id string, userID *uuid.UUID, at time.Time, autoResolved bool) error
}

// alertsAccessChecker is the read-only slice of db.AccessRepo we use to gate
// per-rover authorization.
type alertsAccessChecker interface {
	RoleFor(ctx context.Context, userID uuid.UUID, roverID string) (string, error)
}

// alertsPublisher narrows *nats.Conn so tests can verify publishes via a stub.
// The real impl is satisfied by *nats.Conn.
type alertsPublisher interface {
	Publish(subj string, data []byte) error
}

// Compile-time assertion that *nats.Conn implements alertsPublisher.
var _ alertsPublisher = (*nats.Conn)(nil)

// alertWireRow is the on-the-wire shape returned by GET /api/alerts. Optional
// fields are JSON-omitted when zero. Timestamps are RFC3339 UTC for symmetry
// with /api/audit.
type alertWireRow struct {
	ID             string          `json:"id"`
	RoverID        string          `json:"roverId"`
	Severity       string          `json:"severity"`
	Source         string          `json:"source"`
	Code           string          `json:"code"`
	Message        string          `json:"message"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	RaisedAt       string          `json:"raisedAt"`
	State          string          `json:"state"`
	AcknowledgedAt string          `json:"acknowledgedAt,omitempty"`
	AcknowledgedBy string          `json:"acknowledgedBy,omitempty"`
	ResolvedAt     string          `json:"resolvedAt,omitempty"`
	ResolvedBy     string          `json:"resolvedBy,omitempty"`
	AutoResolved   bool            `json:"autoResolved,omitempty"`
}

func toAlertWire(a db.AlertRow) alertWireRow {
	w := alertWireRow{
		ID:           a.ID,
		RoverID:      a.RoverID,
		Severity:     a.Severity,
		Source:       a.Source,
		Code:         a.Code,
		Message:      a.Message,
		Metadata:     a.Metadata,
		RaisedAt:     a.RaisedAt.UTC().Format(time.RFC3339),
		State:        a.State,
		AutoResolved: a.AutoResolved,
	}
	if a.AcknowledgedAt != nil {
		w.AcknowledgedAt = a.AcknowledgedAt.UTC().Format(time.RFC3339)
	}
	if a.AcknowledgedBy != nil {
		w.AcknowledgedBy = a.AcknowledgedBy.String()
	}
	if a.ResolvedAt != nil {
		w.ResolvedAt = a.ResolvedAt.UTC().Format(time.RFC3339)
	}
	if a.ResolvedBy != nil {
		w.ResolvedBy = a.ResolvedBy.String()
	}
	return w
}

// hasControlOrAdmin returns true when the access role can mutate alert state
// on a rover. Admins are short-circuited by the caller before invoking this.
func hasControlOrAdmin(role string) bool {
	return role == "control" || role == "admin"
}

// newAlertsListHandler serves GET /api/alerts. Admins may omit `rover` to list
// across the fleet; non-admins must scope by a rover they have any role on.
// The handler returns state=active rows only (history is a future scope).
func newAlertsListHandler(repo alertsRepo, access alertsAccessChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := authkit.UserFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()
		rover := q.Get("rover")

		switch {
		case u.IsAdmin:
			// no extra filtering
		case rover != "":
			role, err := access.RoleFor(r.Context(), u.ID, rover)
			if err != nil {
				http.Error(w, "access lookup: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if role == "" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		default:
			http.Error(w, "rover query param required for non-admin", http.StatusBadRequest)
			return
		}

		// `limit` is parsed for forward-compat parity with /api/audit but the
		// repo currently returns the active set unbounded. Cap is enforced
		// only when active rows exceed it; typical fleets surface tens at most.
		limit := 100
		if s := q.Get("limit"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
				return
			}
			if n > 500 {
				n = 500
			}
			limit = n
		}

		rows, err := repo.ListActive(r.Context(), rover)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(rows) > limit {
			rows = rows[:limit]
		}
		out := make([]alertWireRow, 0, len(rows))
		for _, a := range rows {
			out = append(out, toAlertWire(a))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// newAlertsAckHandler serves POST /api/alerts/{id}/acknowledge.
// Requires control+ on the alert's rover; admins bypass the access check.
// On success, publishes AlertAcknowledged on the rover's subject so the
// rover's local SQLite mirror converges.
func newAlertsAckHandler(repo alertsRepo, access alertsAccessChecker, nc alertsPublisher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := authkit.UserFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		alert, err := repo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !u.IsAdmin {
			role, err := access.RoleFor(r.Context(), u.ID, alert.RoverID)
			if err != nil {
				http.Error(w, "access lookup: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !hasControlOrAdmin(role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		now := time.Now().UTC()
		if err := repo.Acknowledge(r.Context(), id, u.ID, now); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Race: row vanished between GetByID and Acknowledge.
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ev := &waypointv1.AlertAcknowledged{
			Id:     id,
			UserId: u.ID.String(),
			T:      timestamppb.New(now),
		}
		body, _ := proto.Marshal(ev)
		_ = nc.Publish("waypoint."+alert.RoverID+".alert.acknowledged", body)

		w.WriteHeader(http.StatusNoContent)
	})
}

// newAlertsResolveHandler serves POST /api/alerts/{id}/resolve.
// Same authorization as acknowledge. Body is ignored — `reason` is always
// "user:<uuid>" so the resolution is attributable. auto_resolved is always
// false from this endpoint; auto-resolve flows go through the in-process
// mirror, not the public API.
func newAlertsResolveHandler(repo alertsRepo, access alertsAccessChecker, nc alertsPublisher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := authkit.UserFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		alert, err := repo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !u.IsAdmin {
			role, err := access.RoleFor(r.Context(), u.ID, alert.RoverID)
			if err != nil {
				http.Error(w, "access lookup: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !hasControlOrAdmin(role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		now := time.Now().UTC()
		userID := u.ID
		if err := repo.Resolve(r.Context(), id, &userID, now, false); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reason := "user:" + u.ID.String()
		ev := &waypointv1.AlertResolved{
			Id:           id,
			Reason:       reason,
			AutoResolved: false,
			T:            timestamppb.New(now),
		}
		body, _ := proto.Marshal(ev)
		_ = nc.Publish("waypoint."+alert.RoverID+".alert.resolved", body)

		w.WriteHeader(http.StatusNoContent)
	})
}
