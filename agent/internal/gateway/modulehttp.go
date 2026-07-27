package gateway

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// ModuleStaticHandler serves /module/<id>/static/* from
// <mountRoot>/<id>/dashboard/*. In production mountRoot is where the agent
// mounts module images read-only (modules.DefaultModuleMountRoot).
type ModuleStaticHandler struct {
	mountRoot string
}

func NewModuleStaticHandler(mountRoot string) *ModuleStaticHandler {
	return &ModuleStaticHandler{mountRoot: mountRoot}
}

func (h *ModuleStaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/module/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	id, after, ok := strings.Cut(rest, "/static/")
	if !ok || id == "" {
		http.NotFound(w, r)
		return
	}
	moduleRoot := filepath.Join(h.mountRoot, id, "dashboard")
	target := filepath.Clean(filepath.Join(moduleRoot, after))
	if !strings.HasPrefix(target, filepath.Clean(moduleRoot)+string(filepath.Separator)) &&
		target != filepath.Clean(moduleRoot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, target)
}

// ModuleProxyRegistry tracks which modules currently bind a reverse-proxy
// port. The reconciler updates this from the attached set; the HTTP handler
// reads it on each request.
type ModuleProxyRegistry struct {
	mu    sync.RWMutex
	ports map[string]uint16
}

func NewModuleProxyRegistry() *ModuleProxyRegistry {
	return &ModuleProxyRegistry{ports: map[string]uint16{}}
}

func (r *ModuleProxyRegistry) Set(moduleID string, port uint16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ports[moduleID] = port
}

func (r *ModuleProxyRegistry) Clear(moduleID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ports, moduleID)
}

func (r *ModuleProxyRegistry) Get(moduleID string) (uint16, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.ports[moduleID]
	return p, ok
}

// ModuleProxyHandler reverse-proxies /module/<id>/* to 127.0.0.1:<port>.
type ModuleProxyHandler struct {
	reg *ModuleProxyRegistry
}

func NewModuleProxyHandler(reg *ModuleProxyRegistry) *ModuleProxyHandler {
	return &ModuleProxyHandler{reg: reg}
}

func (h *ModuleProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/module/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	id, after, _ := strings.Cut(rest, "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	port, ok := h.reg.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(int(port)))
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = "/" + after
			pr.Out.Host = target.Host
		},
	}
	rp.ServeHTTP(w, r)
}
