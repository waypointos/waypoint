package gateway

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModuleStatic_ServesFromMountedRaw(t *testing.T) {
	mountRoot := t.TempDir()
	dashDir := filepath.Join(mountRoot, "power-monitor", "dashboard")
	require.NoError(t, os.MkdirAll(dashDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dashDir, "panel.js"), []byte("export default {mount(){}};"), 0o644))

	h := NewModuleStaticHandler(mountRoot)
	req := httptest.NewRequest("GET", "/module/power-monitor/static/panel.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "export default")
}

func TestModuleStatic_404WhenModuleNotAttached(t *testing.T) {
	mountRoot := t.TempDir()
	h := NewModuleStaticHandler(mountRoot)
	req := httptest.NewRequest("GET", "/module/missing/static/panel.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestModuleStatic_404WhenFileMissing(t *testing.T) {
	mountRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mountRoot, "x", "dashboard"), 0o755))
	h := NewModuleStaticHandler(mountRoot)
	req := httptest.NewRequest("GET", "/module/x/static/missing.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestModuleStatic_RejectsPathTraversal(t *testing.T) {
	mountRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mountRoot, "x", "dashboard"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mountRoot, "secret"), []byte("nope"), 0o644))
	h := NewModuleStaticHandler(mountRoot)
	req := httptest.NewRequest("GET", "/module/x/static/../../secret", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusOK, rec.Code)
}

func TestModuleProxy_ForwardsToLoopback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Path", r.URL.Path)
		w.Write([]byte("pong"))
	}))
	defer upstream.Close()
	upstreamPort := mustPort(t, upstream.URL)

	reg := NewModuleProxyRegistry()
	reg.Set("power-monitor", upstreamPort)
	h := NewModuleProxyHandler(reg)

	req := httptest.NewRequest("GET", "/module/power-monitor/sub/page", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "/sub/page", rec.Header().Get("X-Upstream-Path"))
	require.Equal(t, "pong", rec.Body.String())
}

func TestModuleProxy_404WhenNotRegistered(t *testing.T) {
	reg := NewModuleProxyRegistry()
	h := NewModuleProxyHandler(reg)
	req := httptest.NewRequest("GET", "/module/missing/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func mustPort(t *testing.T, urlStr string) uint16 {
	t.Helper()
	u, err := url.Parse(urlStr)
	require.NoError(t, err)
	p, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return uint16(p)
}
