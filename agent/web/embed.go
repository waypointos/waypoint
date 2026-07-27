package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded dashboard build with a single-page-app fallback.
//
// The dashboard is a client-routed SPA: paths like /rover/sojourner/logs exist
// only in the browser router, not as files on disk. A plain http.FileServer
// 404s those on a hard reload (the browser asks the server for that exact
// path). So we serve the real file when one backs the request and otherwise
// fall back to index.html, letting the client router resolve the route.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		if info, err := fs.Stat(sub, p); err == nil && !info.IsDir() {
			// A real asset (js, css, svg, fonts) backs this path — serve it.
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path: a client-router route. Serve index.html so a hard
		// reload of a deep link resolves instead of 404ing. The browser keeps
		// the original URL, so the SPA still sees the route it was loaded at.
		index, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, "dashboard index not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}
