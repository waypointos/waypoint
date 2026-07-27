package authkit

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

type ctxKey struct{ name string }

var userKey = ctxKey{"user"}

type CurrentUser struct {
	ID      uuid.UUID
	Email   string
	IsAdmin bool
}

func UserFrom(ctx context.Context) (*CurrentUser, bool) {
	u, ok := ctx.Value(userKey).(*CurrentUser)
	return u, ok
}

func WithUser(ctx context.Context, u *CurrentUser) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// RequireUser loads the session, fetches the user from DB, attaches to ctx.
// Returns 401 if no session or user lookup fails.
func RequireUser(session *SessionCodec, users *db.UsersRepo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := session.Get(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		uid, err := uuid.Parse(sess.UserID)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := users.GetByID(r.Context(), uid)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, &CurrentUser{
			ID: u.ID, Email: u.Email, IsAdmin: u.IsAdmin,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireUserHTML is the HTML/asset counterpart to RequireUser. On a missing
// or invalid session it 302s to /auth/login with the original URL stashed in
// the "next" query parameter, so the user lands back where they started after
// signing in.
func RequireUserHTML(session *SessionCodec, users *db.UsersRepo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect := func() {
			http.Redirect(w, r, "/auth/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		}
		sess, err := session.Get(r)
		if err != nil {
			redirect()
			return
		}
		uid, err := uuid.Parse(sess.UserID)
		if err != nil {
			redirect()
			return
		}
		u, err := users.GetByID(r.Context(), uid)
		if err != nil {
			redirect()
			return
		}
		ctx := context.WithValue(r.Context(), userKey, &CurrentUser{
			ID: u.ID, Email: u.Email, IsAdmin: u.IsAdmin,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFrom(r.Context())
		if !ok || !u.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ParseUserID parses a string user id from a session payload. Exposed so
// callers outside the authkit package (e.g. the SPA gate router) can perform
// the same validation without duplicating uuid logic.
func ParseUserID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
