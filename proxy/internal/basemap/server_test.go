package basemap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFixture copies testdata/tiny.pmtiles into dir as basemap.pmtiles, the
// stem the server expects. The fixture (go-pmtiles' test_fixture_1) holds a
// single tile at z/x/y 0/0/0 and nothing above zoom 0.
func writeFixture(t *testing.T, dir string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "tiny.pmtiles"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "basemap.pmtiles"), b, 0o644))
}

func TestServer_GetTileHit(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	srv, err := New(dir)
	require.NoError(t, err)
	defer srv.Close()

	status, _, body := srv.Get(context.Background(), "/basemap/0/0/0.mvt")
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, body)
}

func TestServer_GetMissReturns204(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	srv, err := New(dir)
	require.NoError(t, err)
	defer srv.Close()

	// Above the fixture's max zoom: a guaranteed miss.
	status, _, body := srv.Get(context.Background(), "/basemap/5/0/0.mvt")
	require.Equal(t, http.StatusNoContent, status)
	require.Empty(t, body)
}

func TestServer_ServeHTTPSetsContentType(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	srv, err := New(dir)
	require.NoError(t, err)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/basemap/0/0/0.mvt", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, rec.Body.Bytes())
	require.Contains(t, rec.Header().Get("Content-Type"), "protobuf")
}
