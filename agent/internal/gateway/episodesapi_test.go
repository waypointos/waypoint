package gateway

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/waypointos/waypoint/agent/internal/recorder"
)

type fakeEpisodes struct {
	dir    string
	listed []*recorder.Sidecar
}

func (f *fakeEpisodes) List() ([]*recorder.Sidecar, error) { return f.listed, nil }
func (f *fakeEpisodes) Path(id string) (string, error) {
	return filepath.Join(f.dir, id+".mcap"), nil
}
func (f *fakeEpisodes) Delete(id string) error {
	return os.Remove(filepath.Join(f.dir, id+".mcap"))
}

func newTestGateway(t *testing.T) *Gateway {
	t.Helper()
	return New("nats://x", "rover", "tok", "", nil, nil, "", nil)
}

// authedRequest serves a request with a local peer IP and a matching bootstrap
// token, satisfying localAuthorized.
func authedRequest(t *testing.T, g *Gateway, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path+"?token=tok", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rr := httptest.NewRecorder()
	g.HTTPHandler().ServeHTTP(rr, req)
	return rr
}

func TestEpisodesListAuthAndPayload(t *testing.T) {
	g := newTestGateway(t)
	dir := t.TempDir()
	g.episodes = &fakeEpisodes{dir: dir, listed: []*recorder.Sidecar{{EpisodeID: "ep-x", TaskLabel: "demo"}}}

	// Unauthenticated: 401.
	rr := httptest.NewRecorder()
	g.HTTPHandler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/episodes", nil))
	require.Equal(t, 401, rr.Code)

	// Authenticated: JSON list.
	rr = authedRequest(t, g, "GET", "/api/episodes", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `"ep-x"`)
	require.Contains(t, rr.Body.String(), `"demo"`)
}

func TestEpisodeDownloadStreamsFile(t *testing.T) {
	g := newTestGateway(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ep-y.mcap"), []byte("MCAPDATA"), 0o644))
	g.episodes = &fakeEpisodes{dir: dir}

	rr := authedRequest(t, g, "GET", "/api/episodes/ep-y/download", nil)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "MCAPDATA", rr.Body.String())
	require.Contains(t, rr.Header().Get("Content-Disposition"), `ep-y.mcap`)
}

// The dashboard episode player reads .mcap files with Range requests; the
// handler must keep answering 206 with exact slices.
func TestEpisodeDownloadServesRanges(t *testing.T) {
	g := newTestGateway(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ep-r.mcap"), []byte("0123456789"), 0o644))
	g.episodes = &fakeEpisodes{dir: dir}

	req := httptest.NewRequest("GET", "/api/episodes/ep-r/download?token=tok", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("Range", "bytes=2-5")
	rr := httptest.NewRecorder()
	g.HTTPHandler().ServeHTTP(rr, req)

	require.Equal(t, 206, rr.Code)
	require.Equal(t, "2345", rr.Body.String())
	require.Equal(t, "bytes 2-5/10", rr.Header().Get("Content-Range"))
	require.Equal(t, "bytes", rr.Header().Get("Accept-Ranges"))

	// Range requests do not bypass local auth.
	unauth := httptest.NewRequest("GET", "/api/episodes/ep-r/download", nil)
	unauth.Header.Set("Range", "bytes=0-3")
	rr = httptest.NewRecorder()
	g.HTTPHandler().ServeHTTP(rr, unauth)
	require.Equal(t, 401, rr.Code)
}

func TestEpisodeDelete(t *testing.T) {
	g := newTestGateway(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "ep-z.mcap")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	g.episodes = &fakeEpisodes{dir: dir}

	rr := authedRequest(t, g, "DELETE", "/api/episodes/ep-z", nil)
	require.Equal(t, 200, rr.Code)
	_, err := os.Stat(p)
	require.True(t, os.IsNotExist(err))
}
