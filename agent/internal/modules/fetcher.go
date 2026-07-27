package modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Fetcher struct {
	client *http.Client
}

func NewFetcher(client *http.Client) *Fetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &Fetcher{client: client}
}

// Fetch downloads url, verifies its sha256 matches wantHex (lowercase hex
// when non-empty), and writes the bytes atomically to dest. The destination
// directory is created if missing. On any error, the destination is left
// untouched. The url may be a file:// URL for dev-image local overrides.
func (f *Fetcher) Fetch(ctx context.Context, url, wantHex, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("fetcher: mkdir: %w", err)
	}
	body, err := f.open(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".raw-tmp-*")
	if err != nil {
		return fmt.Errorf("fetcher: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), body); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("fetcher: copy: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("fetcher: close temp: %w", err)
	}
	if wantHex != "" {
		gotHex := hex.EncodeToString(h.Sum(nil))
		if gotHex != wantHex {
			cleanup()
			return fmt.Errorf("fetcher: sha256 mismatch: got %s want %s", gotHex, wantHex)
		}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		cleanup()
		return fmt.Errorf("fetcher: rename: %w", err)
	}
	return nil
}

func (f *Fetcher) open(ctx context.Context, url string) (io.ReadCloser, error) {
	if strings.HasPrefix(url, "file://") {
		return os.Open(strings.TrimPrefix(url, "file://"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetcher: new request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetcher: do: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetcher: HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}
