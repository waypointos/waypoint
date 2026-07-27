package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"

	"github.com/waypointos/waypoint/proxy/internal/audit"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

// roverGetterDeleter narrows the rovers repo to what the delete handler needs.
type roverGetterDeleter interface {
	Get(ctx context.Context, id string) (*db.Rover, error)
	Delete(ctx context.Context, id string) error
}

// hubRevoker narrows natshub.Hub to its revoke method.
type hubRevoker interface {
	RevokeAccount(accountPubKey, revokedJWT string) error
}

// newDeleteRoverHandler implements DELETE /api/rovers/{id} (admin-only).
//
// Order matters:
//  1. Look up the rover to capture its NATS account pubkey + name.
//  2. Mint a revoked account JWT and push it to the hub — this immediately
//     boots any active rover connection and rejects re-auth.
//  3. Publish a RoverDeleted audit event while rover_id still references a
//     real row. The subscriber persists audit_events asynchronously.
//  4. DELETE the DB row. FK cascades drop user_rover_access, image history,
//     and alerts; audit_events survives (rover_id is free-text).
//
// If the NATS revoke fails we abort before touching the DB so the operator
// can retry without ending up with a half-deleted rover that's still
// authenticated. If the audit publish fails we proceed — auditing is
// best-effort by design (the rest of the proxy treats it the same way).
func newDeleteRoverHandler(rovers roverGetterDeleter, op *operator.Operator, hub hubRevoker, nc *nats.Conn, conns roverConnRemover, rpc roverRPCRegistry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := authkit.UserFrom(r.Context())
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing rover id", http.StatusBadRequest)
			return
		}

		rover, err := rovers.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		revokedJWT, err := op.MintRevokedAccountJWT(rover.AccountPubKey)
		if err != nil {
			http.Error(w, "mint revoked jwt: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := hub.RevokeAccount(rover.AccountPubKey, revokedJWT); err != nil {
			http.Error(w, "revoke nats account: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if nc != nil {
			pub := audit.NewPublisher(nc, audit.Actor{ID: u.ID.String(), Email: u.Email}, "proxy-api")
			pub.LogRoverDeleted(rover.ID, rover.Name, rover.AccountPubKey)
		}

		if err := rovers.Delete(r.Context(), rover.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if rpc != nil {
			rpc.Remove(rover.ID)
		}
		if conns != nil {
			conns.Remove(rover.ID)
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
