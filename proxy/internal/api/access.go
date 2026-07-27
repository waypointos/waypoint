package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
)

type accessWriter interface {
	Grant(ctx context.Context, userID uuid.UUID, roverID, role string, grantedBy uuid.UUID) error
	Revoke(ctx context.Context, userID uuid.UUID, roverID string) error
}

func newGrantHandler(a accessWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := authkit.UserFrom(r.Context())
		roverID := r.PathValue("id")
		var body struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		uid, err := uuid.Parse(body.UserID)
		if err != nil {
			http.Error(w, "bad user id", 400)
			return
		}
		switch body.Role {
		case "monitor", "control", "admin":
		default:
			http.Error(w, "bad role", 400)
			return
		}
		if err := a.Grant(r.Context(), uid, roverID, body.Role, u.ID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func newRevokeHandler(a accessWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roverID := r.PathValue("id")
		userID, err := uuid.Parse(r.PathValue("userId"))
		if err != nil {
			http.Error(w, "bad user id", 400)
			return
		}
		if err := a.Revoke(r.Context(), userID, roverID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
