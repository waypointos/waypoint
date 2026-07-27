package basemap

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
)

var errOffline = errors.New("basemap: offline")

// Fetcher fills cache misses (implemented over NATS against the proxy).
type Fetcher interface {
	Fetch(z, x, y uint32) (data []byte, encoding string, found bool, err error)
}

var tilePath = regexp.MustCompile(`^/basemap/(\d+)/(\d+)/(\d+)\.mvt$`)

type Handler struct {
	cache *Cache
	fetch Fetcher
}

func NewHandler(c *Cache, f Fetcher) *Handler { return &Handler{cache: c, fetch: f} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m := tilePath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	z := atoi(m[1])
	x := atoi(m[2])
	y := atoi(m[3])

	if b, ok := h.cache.Load(z, x, y); ok {
		writeTile(w, b, "gzip") // cached tiles are always stored gzip-encoded
		return
	}

	data, enc, found, err := h.fetch.Fetch(z, x, y)
	if err != nil || !found {
		w.WriteHeader(http.StatusNoContent) // offline or out-of-coverage → blank
		return
	}
	// Only cache gzip tiles so the hit path can serve a fixed encoding; upstream
	// (Protomaps MVT) is always gzip, so this caches in practice.
	if enc == "gzip" {
		_ = h.cache.Store(z, x, y, data)
	}
	writeTile(w, data, enc)
}

func writeTile(w http.ResponseWriter, data []byte, encoding string) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

func atoi(s string) uint32 { n, _ := strconv.Atoi(s); return uint32(n) }
