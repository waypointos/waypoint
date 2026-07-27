package router

import (
	"net/http"
	"net/url"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

// Router wraps *http.ServeMux and forces every route to declare its auth
// posture. There is intentionally no plain Handle method: every registration
// goes through Public, RequireUser, RequireUserHTML, or SPAGate. Adding a
// route without picking a posture should be impossible.
type Router struct {
	mux   *http.ServeMux
	sess  *authkit.SessionCodec
	users *db.UsersRepo
}

func New(sess *authkit.SessionCodec, users *db.UsersRepo) *Router {
	return &Router{mux: http.NewServeMux(), sess: sess, users: users}
}

// Handler returns the underlying mux. Wrap with TrustForwarded at the top.
func (r *Router) Handler() http.Handler { return r.mux }

// Public registers an unauthenticated route. The caller is asserting this path
// is safe to expose (login, healthz, favicon, landing assets).
func (r *Router) Public(pattern string, h http.Handler) {
	r.mux.Handle(pattern, h)
}

// RequireUser gates JSON/WS routes. 401 on missing or invalid session.
func (r *Router) RequireUser(pattern string, h http.Handler) {
	r.mux.Handle(pattern, authkit.RequireUser(r.sess, r.users, h))
}

// RequireUserHTML gates HTML/asset routes. 302 to /auth/login?next=<uri> on
// missing or invalid session.
func (r *Router) RequireUserHTML(pattern string, h http.Handler) {
	r.mux.Handle(pattern, authkit.RequireUserHTML(r.sess, r.users, h))
}

// SPAGate owns the catch-all at "/". It implements the SPA-shell access model:
//
//   - Authenticated user → authed handler (the SPA, which owns its own routing).
//   - Anonymous user at "/"        → anon handler (the landing page).
//   - Anonymous user at deep link  → 302 to /auth/login?next=<original URI>.
//
// ServeMux longest-prefix matching means routes explicitly registered via
// Public, RequireUser, or RequireUserHTML (e.g. /api/..., /assets/..., /auth/...)
// are matched first; SPAGate sees only the unclaimed paths.
//
// This is the only way to register "/", and registering "/" without going
// through SPAGate is impossible by design.
func (r *Router) SPAGate(authed, anon http.Handler) {
	r.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.hasSession(req) {
			authed.ServeHTTP(w, req)
			return
		}
		if req.URL.Path == "/" {
			anon.ServeHTTP(w, req)
			return
		}
		http.Redirect(w, req, "/auth/login?next="+url.QueryEscape(req.URL.RequestURI()), http.StatusFound)
	}))
}

// hasSession returns true iff the request carries a valid session cookie that
// resolves to a known user. Used by SPAGate to branch on authentication state
// without panicking on a stale or malformed cookie.
func (r *Router) hasSession(req *http.Request) bool {
	sess, err := r.sess.Get(req)
	if err != nil {
		return false
	}
	uid, err := authkit.ParseUserID(sess.UserID)
	if err != nil {
		return false
	}
	if _, err := r.users.GetByID(req.Context(), uid); err != nil {
		return false
	}
	return true
}
