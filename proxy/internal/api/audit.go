package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

// auditLister narrows db.AuditRepo to the subset newAuditListHandler needs.
// A small interface keeps tests free of a real Postgres dependency when the
// authorization or query-parsing branches can be exercised without DB I/O.
type auditLister interface {
	List(ctx context.Context, f db.AuditFilter) ([]db.AuditRow, error)
}

// auditAccessChecker is the read-only slice of db.AccessRepo used here.
type auditAccessChecker interface {
	RoleFor(ctx context.Context, userID uuid.UUID, roverID string) (string, error)
}

// auditWireRow is the on-the-wire shape returned by GET /api/audit. We hoist
// it to the package scope so tests can decode responses against the same type.
type auditWireRow struct {
	ID          string          `json:"id"`
	OccurredAt  string          `json:"occurredAt"`
	Source      string          `json:"source"`
	ActorUserID string          `json:"actorUserId"`
	ActorEmail  string          `json:"actorEmail"`
	RoverID     string          `json:"roverId"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
}

func newAuditListHandler(auditRepo auditLister, access auditAccessChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := authkit.UserFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()
		rover := q.Get("rover")
		sinceStr := q.Get("since")
		limitStr := q.Get("limit")

		// Authorization. Admins see everything. Non-admins must scope by rover
		// they have access to; cross-rover listing is admin-only because we have
		// no per-rover paged UI yet.
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

		f := db.AuditFilter{RoverID: rover, Limit: 500}
		if sinceStr != "" {
			t, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				http.Error(w, "since must be RFC3339", http.StatusBadRequest)
				return
			}
			f.SinceTime = &t
		}
		if limitStr != "" {
			n, err := strconv.Atoi(limitStr)
			if err != nil || n <= 0 {
				http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
				return
			}
			if n > 1000 {
				n = 1000
			}
			f.Limit = n
		}

		rows, err := auditRepo.List(r.Context(), f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := make([]auditWireRow, 0, len(rows))
		for _, row := range rows {
			wr := auditWireRow{
				ID:         row.ID.String(),
				OccurredAt: row.OccurredAt.UTC().Format(time.RFC3339),
				Source:     row.Source,
				ActorEmail: row.ActorEmail,
				RoverID:    row.RoverID,
				Kind:       row.Kind,
				Payload:    row.Payload,
			}
			if row.ActorUserID != nil {
				wr.ActorUserID = row.ActorUserID.String()
			}
			if len(wr.Payload) == 0 {
				wr.Payload = json.RawMessage(`{}`)
			}
			out = append(out, wr)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}
