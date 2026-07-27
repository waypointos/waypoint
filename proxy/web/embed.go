package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded dist directory rooted at dist/. Use for serving
// individual files (e.g. favicon.svg) from outside this package.
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil
	}
	return sub
}

// Handler serves files from the embedded dist tree. Use for paths that map
// 1:1 to real files (e.g. /assets/, /favicon.svg); deep SPA routes must use
// ShellHandler so React Router can claim them client-side.
func Handler() http.Handler {
	sub := DistFS()
	if sub == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("dashboard not embedded yet. run make embed-dashboard."))
		})
	}
	return http.FileServer(http.FS(sub))
}

// ShellHandler always serves dist/index.html, regardless of request path. It's
// the SPA fallback: deep links like /admin or /rover/abc are not real files,
// so FileServer would 404. React Router reads window.location after boot and
// renders the matching route. no-store keeps a stale shell from referencing
// hashed asset paths that no longer exist after a redeploy.
func ShellHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := DistFS()
		if sub == nil {
			http.Error(w, "dashboard not embedded yet. run make embed-dashboard.", http.StatusInternalServerError)
			return
		}
		f, err := sub.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	})
}
