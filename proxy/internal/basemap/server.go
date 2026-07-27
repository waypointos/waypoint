// Package basemap serves the dashboard's map tiles same-origin from a
// self-hosted PMTiles archive (an S3-compatible bucket in production, a local
// directory in dev). It exposes XYZ vector tiles at /basemap/{z}/{x}/{y}.mvt and
// a Get() used by both the HTTP handler and the per-rover NATS responder.
package basemap

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/protomaps/go-pmtiles/pmtiles"

	// Register the gocloud S3 driver so pmtiles.OpenBucket can read s3:// sources.
	// go-pmtiles registers blob drivers only in its CLI, not the library package.
	_ "gocloud.dev/blob/s3blob"
)

// archive is the PMTiles file stem; callers supply paths of the form
// /basemap/{z}/{x}/{y}.mvt, so the archive must be named basemap.pmtiles.
const archive = "basemap"

type Server struct {
	inner  *pmtiles.Server
	bucket pmtiles.Bucket
}

// New opens the basemap.pmtiles archive named by source. A bare path is a local
// directory (dev); a scheme URL is passed through to go-pmtiles — notably
// `s3://<bucket>`, whose endpoint/region/credentials come from the standard
// AWS_* environment (set by Railway's bucket connection).
func New(source string) (*Server, error) {
	ctx := context.Background()
	if !strings.Contains(source, "://") {
		source = "file://" + source
	}
	bucket, err := pmtiles.OpenBucket(ctx, source, "")
	if err != nil {
		return nil, err
	}
	// Silence go-pmtiles' internal per-fetch logging; it would flood proxy logs.
	inner, err := pmtiles.NewServerWithBucket(bucket, "", log.New(io.Discard, "", 0), 256, "")
	if err != nil {
		bucket.Close()
		return nil, err
	}
	inner.Start()
	return &Server{inner: inner, bucket: bucket}, nil
}

// Get returns (status, headers, body) for a tile path like "/basemap/{z}/{x}/{y}.mvt".
// A miss returns 204 with an empty body.
func (s *Server) Get(ctx context.Context, path string) (int, map[string]string, []byte) {
	status, headers, body := s.inner.Get(ctx, path)
	if status == http.StatusNotFound {
		return http.StatusNoContent, headers, nil
	}
	return status, headers, body
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, headers, body := s.Get(r.Context(), r.URL.Path)
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	// go-pmtiles sets Content-Type on hits; default it only for a served tile.
	if status == http.StatusOK && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/x-protobuf")
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *Server) Close() error { return s.bucket.Close() }
