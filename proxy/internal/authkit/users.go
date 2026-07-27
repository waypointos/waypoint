package authkit

import (
	"encoding/json"
	"net/http"
	"time"
)

type userAccessGrant struct {
	RoverID string `json:"roverId"`
	Role    string `json:"role"`
}

type userListEntry struct {
	ID          string            `json:"id"`
	Email       string            `json:"email"`
	IsAdmin     bool              `json:"isAdmin"`
	CreatedAt   string            `json:"createdAt,omitempty"`
	LastLoginAt string            `json:"lastLoginAt,omitempty"`
	Access      []userAccessGrant `json:"access"`
}

// ListUsers returns every user and their per-rover grants. Admin-only.
func (d *Deps) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := d.Users.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]userListEntry, 0, len(users))
	for _, u := range users {
		grants, err := d.Access.ListForUser(r.Context(), u.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entry := userListEntry{
			ID:        u.ID.String(),
			Email:     u.Email,
			IsAdmin:   u.IsAdmin,
			CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
			Access:    make([]userAccessGrant, 0, len(grants)),
		}
		if u.LastLoginAt != nil {
			entry.LastLoginAt = u.LastLoginAt.UTC().Format(time.RFC3339)
		}
		for _, g := range grants {
			entry.Access = append(entry.Access, userAccessGrant{RoverID: g.RoverID, Role: g.Role})
		}
		out = append(out, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
