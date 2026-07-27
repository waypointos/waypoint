package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

type roversLister interface {
	List(ctx context.Context) ([]db.Rover, error)
}
type accessLister interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]db.Access, error)
}

func newListHandler(r roversLister, a accessLister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		u, _ := authkit.UserFrom(req.Context())
		all, err := r.List(req.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var grants map[string]string
		if !u.IsAdmin {
			gs, err := a.ListForUser(req.Context(), u.ID)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			grants = make(map[string]string, len(gs))
			for _, g := range gs {
				grants[g.RoverID] = g.Role
			}
		}
		type outItem struct {
			ID           string     `json:"id"`
			Name         string     `json:"name"`
			Role         string     `json:"role"`
			LastSeen     *time.Time `json:"lastSeen,omitempty"`
			ImageVersion *string    `json:"imageVersion,omitempty"`
		}
		var out []outItem
		for _, rv := range all {
			role := "admin"
			if !u.IsAdmin {
				rl, ok := grants[rv.ID]
				if !ok {
					continue
				}
				role = rl
			}
			out = append(out, outItem{
				ID: rv.ID, Name: rv.Name, Role: role,
				LastSeen: rv.LastSeenAt, ImageVersion: rv.ImageVersion,
			})
		}
		if out == nil {
			out = []outItem{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}
