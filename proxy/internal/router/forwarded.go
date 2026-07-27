package router

import "net/http"

// TrustForwarded promotes X-Forwarded-Proto and X-Forwarded-Host into the
// request's Scheme and Host. Applied at the top of the handler chain so any
// downstream code that inspects r.URL.Scheme or r.Host sees the public-facing
// values rather than the upstream-internal HTTP values from Railway.
//
// Unconditional trust is fine here: every public path to the app terminates
// TLS at Railway and arrives over the internal network. No path bypasses it.
func TrustForwarded(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			r.URL.Scheme = proto
		}
		if host := r.Header.Get("X-Forwarded-Host"); host != "" {
			r.Host = host
		}
		next.ServeHTTP(w, r)
	})
}
