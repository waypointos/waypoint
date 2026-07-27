package authkit

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"

	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

type Deps struct {
	WorkOS        Authenticator
	Inviter       Inviter
	Users         *db.UsersRepo
	Session       *SessionCodec
	StateCodec    *StateCodec
	Access        *db.AccessRepo
	Rovers        *db.RoversRepo
	Operator      *operator.Operator
	PublicNATSURL string
	CookieSecure  bool
}

func (d *Deps) Login(w http.ResponseWriter, r *http.Request) {
	stateBuf := make([]byte, 16)
	_, _ = rand.Read(stateBuf)
	state := base64.RawURLEncoding.EncodeToString(stateBuf)

	next := SanitizeNext(r.URL.Query().Get("next"))

	if err := d.StateCodec.Set(w, StatePayload{State: state, Next: next}); err != nil {
		log.Printf("state set: %v", err)
		http.Error(w, "state error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, d.WorkOS.AuthorizationURL(state), http.StatusFound)
}

func (d *Deps) Callback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value == "" {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	payload, err := d.StateCodec.Decode(stateCookie.Value)
	if err != nil {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	if payload.State != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	res, err := d.WorkOS.Authenticate(r.Context(), code)
	if err != nil {
		log.Printf("workos auth: %v", err)
		http.Error(w, "auth failed", http.StatusUnauthorized)
		return
	}

	user, err := d.Users.UpsertFromWorkOS(r.Context(), res.WorkOSUserID, res.Email)
	if err != nil {
		log.Printf("user upsert: %v", err)
		http.Error(w, "user upsert failed", http.StatusInternalServerError)
		return
	}

	if err := d.Session.Set(w, Session{UserID: user.ID.String()}); err != nil {
		log.Printf("session set: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	d.StateCodec.Clear(w)
	http.Redirect(w, r, SanitizeNext(payload.Next), http.StatusFound)
}

func (d *Deps) Logout(w http.ResponseWriter, r *http.Request) {
	d.Session.Clear(w)
	w.WriteHeader(http.StatusNoContent)
}

func (d *Deps) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resp := map[string]any{
		"id":      u.ID.String(),
		"email":   u.Email,
		"isAdmin": u.IsAdmin,
		"mode":    "proxy",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
