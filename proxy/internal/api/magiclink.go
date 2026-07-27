package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nkeys"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/magiclink"
)

type roleLookup interface {
	EffectiveRole(ctx context.Context, userID uuid.UUID, roverID string, isAdmin bool) (string, error)
}

type roverGet interface {
	Get(ctx context.Context, id string) (*db.Rover, error)
}

func newMagicLinkHandler(role roleLookup, rovers roverGet, signer nkeys.KeyPair) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := authkit.UserFrom(r.Context())
		roverID := r.PathValue("id")
		var body struct {
			LocalURL string `json:"localUrl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.LocalURL == "" {
			http.Error(w, "bad request", 400)
			return
		}

		eff, err := role.EffectiveRole(r.Context(), u.ID, roverID, u.IsAdmin)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if eff == "" {
			http.Error(w, "forbidden", 403)
			return
		}

		if _, err := rovers.Get(r.Context(), roverID); err != nil {
			http.Error(w, "no such rover", 404)
			return
		}

		tok, err := magiclink.Mint(signer, magiclink.Claims{
			Sub: u.ID.String(), Email: u.Email,
			RoverID: roverID, Role: eff,
			ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		dest, err := url.Parse(body.LocalURL)
		if err != nil {
			http.Error(w, "bad localUrl", 400)
			return
		}
		dest.Path = "/login"
		q := dest.Query()
		q.Set("token", tok)
		dest.RawQuery = q.Encode()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": dest.String()})
	})
}
