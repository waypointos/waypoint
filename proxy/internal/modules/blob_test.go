package modules

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiskBlobStore_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewDiskBlobStore(dir)
	path, err := store.Put(context.Background(), "x", "0.1.0", bytes.NewReader([]byte("raw bytes")))
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(path))

	rc, err := store.Get(context.Background(), path)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, []byte("raw bytes"), got)
}

func TestDiskBlobStore_PathOutsideRootRejected(t *testing.T) {
	store := NewDiskBlobStore(t.TempDir())
	_, err := store.Get(context.Background(), "/etc/shadow")
	require.Error(t, err)
}

func TestDiskBlobStore_Delete(t *testing.T) {
	store := NewDiskBlobStore(t.TempDir())
	path, err := store.Put(context.Background(), "x", "0.1.0", bytes.NewReader([]byte("raw")))
	require.NoError(t, err)
	require.NoError(t, store.Delete(context.Background(), path))
	_, err = store.Get(context.Background(), path)
	require.Error(t, err)
	// Deleting an already-missing blob is not an error.
	require.NoError(t, store.Delete(context.Background(), path))
	// A path outside the root is still rejected.
	require.Error(t, store.Delete(context.Background(), "/etc/shadow"))
}
