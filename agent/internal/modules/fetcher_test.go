package modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetch_SuccessWritesAtomically(t *testing.T) {
	payload := []byte("this is a fake raw file")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "x", "0.1.0.raw")
	f := NewFetcher(srv.Client())
	err := f.Fetch(context.Background(), srv.URL+"/x.raw", hexSum, dest)
	require.NoError(t, err)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestFetch_Sha256Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("wrong bytes"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "x.raw")
	f := NewFetcher(srv.Client())
	err := f.Fetch(context.Background(), srv.URL+"/x.raw", "0000000000000000000000000000000000000000000000000000000000000000", dest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sha256 mismatch")
	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "destination must not exist after mismatch")
}

func TestFetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "x.raw")
	f := NewFetcher(srv.Client())
	err := f.Fetch(context.Background(), srv.URL+"/x.raw", "", dest)
	require.Error(t, err)
}

func TestFetch_FileURL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.raw")
	require.NoError(t, os.WriteFile(src, []byte("local bytes"), 0o644))
	dest := filepath.Join(dir, "dest.raw")
	f := NewFetcher(nil)
	require.NoError(t, f.Fetch(context.Background(), "file://"+src, "", dest))
	got, _ := os.ReadFile(dest)
	require.Equal(t, []byte("local bytes"), got)
}
