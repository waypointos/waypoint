package modules

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gocloud.dev/blob/memblob"
)

func TestBucketBlobStore_PutAndGet(t *testing.T) {
	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()
	store := NewBucketBlobStore(bucket)

	key, err := store.Put(context.Background(), "so100", "0.1.2", bytes.NewReader([]byte("raw bytes")))
	require.NoError(t, err)
	require.Equal(t, "modules/so100/0.1.2.raw", key)

	rc, err := store.Get(context.Background(), key)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, []byte("raw bytes"), got)
}

func TestBucketBlobStore_RejectsDiskEraPaths(t *testing.T) {
	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()
	store := NewBucketBlobStore(bucket)

	_, err := store.Get(context.Background(), "/var/lib/waypoint-proxy/modules/so100/0.1.1.raw")
	require.ErrorContains(t, err, "re-ingest required")
}

func TestBucketBlobStore_StaticRoundTrip(t *testing.T) {
	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()
	store := NewBucketBlobStore(bucket)

	require.NoError(t, store.PutStatic(context.Background(), "so100", "0.1.2", "panel.js", []byte("panel js")))
	require.NoError(t, store.PutStatic(context.Background(), "so100", "0.1.2", "models/arm/mesh.stl", []byte("mesh")))
	rc, err := store.GetStatic(context.Background(), "so100", "0.1.2", "panel.js")
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, []byte("panel js"), got)

	rcMesh, err := store.GetStatic(context.Background(), "so100", "0.1.2", "models/arm/mesh.stl")
	require.NoError(t, err)
	defer rcMesh.Close()
	gotMesh, err := io.ReadAll(rcMesh)
	require.NoError(t, err)
	require.Equal(t, []byte("mesh"), gotMesh)

	// DeleteStatic clears the whole version tree; empty trees are a no-op.
	require.NoError(t, store.DeleteStatic(context.Background(), "so100", "0.1.2"))
	_, err = store.GetStatic(context.Background(), "so100", "0.1.2", "panel.js")
	require.Error(t, err)
	_, err = store.GetStatic(context.Background(), "so100", "0.1.2", "models/arm/mesh.stl")
	require.Error(t, err)
	require.NoError(t, store.DeleteStatic(context.Background(), "so100", "0.1.2"))
}

func TestBucketBlobStore_Delete(t *testing.T) {
	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()
	store := NewBucketBlobStore(bucket)

	key, err := store.Put(context.Background(), "so100", "0.1.2", bytes.NewReader([]byte("raw")))
	require.NoError(t, err)
	require.NoError(t, store.Delete(context.Background(), key))
	_, err = store.Get(context.Background(), key)
	require.Error(t, err)
	// Already-missing and disk-era paths are both no-ops.
	require.NoError(t, store.Delete(context.Background(), key))
	require.NoError(t, store.Delete(context.Background(), "/var/lib/waypoint-proxy/modules/x/0.1.0.raw"))
}
