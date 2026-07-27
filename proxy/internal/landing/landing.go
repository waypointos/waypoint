package landing

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed all:templates all:static
var assets embed.FS

type Handler struct {
	tmpl   *template.Template
	static http.Handler
}

// New parses the embedded template at startup and prepares a file server for
// the static assets. Panics on parse error (boot-time bug, not runtime).
func New() *Handler {
	tmpl, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		panic("landing template parse: " + err.Error())
	}
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic("landing static fs: " + err.Error())
	}
	return &Handler{tmpl: tmpl, static: http.FileServer(http.FS(sub))}
}

// Index renders the landing page.
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// Static returns an http.Handler that serves files under static/. Caller must
// strip the /landing/static/ prefix.
func (h *Handler) Static() http.Handler { return h.static }
