package basemap

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	data   []byte
	found  bool
	online bool
	calls  int
}

func (f *fakeFetcher) Fetch(z, x, y uint32) (data []byte, encoding string, found bool, err error) {
	f.calls++
	if !f.online {
		return nil, "", false, errOffline
	}
	return f.data, "gzip", f.found, nil
}

func TestHandler_CacheHitSkipsFetch(t *testing.T) {
	c := NewCache(t.TempDir())
	require.NoError(t, c.Store(2, 1, 1, []byte("cached")))
	f := &fakeFetcher{online: true, found: true, data: []byte("fresh")}
	h := NewHandler(c, f)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/basemap/2/1/1.mvt", nil))
	require.Equal(t, 200, rec.Code)
	require.Equal(t, "cached", rec.Body.String())
	require.Equal(t, 0, f.calls)
}

func TestHandler_MissFetchesAndStores(t *testing.T) {
	c := NewCache(t.TempDir())
	f := &fakeFetcher{online: true, found: true, data: []byte("fresh")}
	h := NewHandler(c, f)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/basemap/2/1/1.mvt", nil))
	require.Equal(t, 200, rec.Code)
	require.Equal(t, "fresh", rec.Body.String())
	require.Equal(t, 1, f.calls)

	stored, ok := c.Load(2, 1, 1)
	require.True(t, ok)
	require.Equal(t, []byte("fresh"), stored)
}

func TestHandler_OfflineMissReturnsEmptyTile(t *testing.T) {
	c := NewCache(t.TempDir())
	f := &fakeFetcher{online: false}
	h := NewHandler(c, f)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/basemap/2/1/1.mvt", nil))
	require.Equal(t, 204, rec.Code)
}
