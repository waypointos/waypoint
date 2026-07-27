package authkit

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/waypointos/waypoint/proxy/internal/operator"
)

// NatsCred mints a short-lived NATS session JWT scoped to the caller's rover
// accesses, returning it alongside an ephemeral NKey seed. The dashboard
// presents (jwt, seed) when opening the /ws/nats WebSocket so the embedded
// NATS server can enforce ACLs at the protocol level.
//
// Admins implicitly receive admin role on every rover in the fleet — the
// per-user access table is only consulted for non-admin callers.
func (d *Deps) NatsCred(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var sa []operator.SessionAccess
	if u.IsAdmin {
		rs, err := d.Rovers.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, rv := range rs {
			sa = append(sa, operator.SessionAccess{RoverID: rv.ID, Role: "admin"})
		}
	} else {
		accesses, err := d.Access.ListForUser(r.Context(), u.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, a := range accesses {
			sa = append(sa, operator.SessionAccess{RoverID: a.RoverID, Role: a.Role})
		}
	}

	tok, seed, err := d.Operator.MintSessionUserJWT(u.ID.String(), sa, operator.SessionJWTTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jwt":       tok,
		"seed":      base64.StdEncoding.EncodeToString(seed),
		"expiresAt": time.Now().Add(operator.SessionJWTTTL).Unix(),
		"natsUrl":   d.PublicNATSURL,
	})
}
