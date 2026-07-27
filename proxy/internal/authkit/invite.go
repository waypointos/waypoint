package authkit

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Inviter is the minimal contract the invite handler needs. *WorkOS satisfies it.
// Kept as an interface so tests can inject a stub without hitting the real API.
type Inviter interface {
	SendInvitation(ctx context.Context, email string, inviterUserID string) (InviteResult, error)
}

type inviteRequest struct {
	Email string `json:"email"`
}

type inviteResponse struct {
	Email     string `json:"email"`
	AcceptURL string `json:"acceptUrl,omitempty"`
	EmailSent bool   `json:"emailSent"`
}

// InviteUser triggers a WorkOS invitation email for the given address.
// Admin-only. The row in our `users` table is created lazily on first sign-in
// via UpsertFromWorkOS; we intentionally do NOT pre-create it here.
func (d *Deps) InviteUser(w http.ResponseWriter, r *http.Request) {
	var req inviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}

	if d.Inviter == nil {
		http.Error(w, "invite not configured", http.StatusInternalServerError)
		return
	}

	// Best-effort: surface the inviter's WorkOS ID if we can find it, so the
	// invite shows up attributed in the WorkOS dashboard. Skipping on lookup
	// failure is fine — the invitation still sends.
	var inviterWorkOSID string
	if cur, ok := UserFrom(r.Context()); ok {
		if u, err := d.Users.GetByID(r.Context(), cur.ID); err == nil {
			inviterWorkOSID = u.WorkOSUserID
		}
	}

	res, err := d.Inviter.SendInvitation(r.Context(), req.Email, inviterWorkOSID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(inviteResponse{
		Email:     req.Email,
		AcceptURL: res.AcceptURL,
		EmailSent: res.EmailSent,
	})
}
