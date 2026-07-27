package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	// Register the S3 driver so main can open the Railway bucket by s3:// URL.
	_ "gocloud.dev/blob/s3blob"
)

// bucketPrefix namespaces module artifacts inside the shared proxy bucket
// (basemap tiles live at the root).
const bucketPrefix = "modules/"

// BucketBlobStore is the S3-compatible BlobStore: artifacts survive proxy
// redeploys, which wipe the container disk the DiskBlobStore writes to.
type BucketBlobStore struct {
	bucket *blob.Bucket
}

func NewBucketBlobStore(bucket *blob.Bucket) *BucketBlobStore {
	return &BucketBlobStore{bucket: bucket}
}

func (s *BucketBlobStore) Put(ctx context.Context, moduleID, version string, src io.Reader) (string, error) {
	key := bucketPrefix + moduleID + "/" + version + ".raw"
	w, err := s.bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return "", fmt.Errorf("blob put: writer: %w", err)
	}
	if _, err := io.Copy(w, src); err != nil {
		w.Close()
		return "", fmt.Errorf("blob put: copy: %w", err)
	}
	// gocloud commits the object on Close, so readers never see partial writes.
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("blob put: close: %w", err)
	}
	return key, nil
}

func (s *BucketBlobStore) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	// DB rows written by the disk-era store hold absolute filesystem paths;
	// those blobs died with their container and need a version re-ingest.
	if !strings.HasPrefix(path, bucketPrefix) {
		return nil, errors.New("blob get: not a bucket key (pre-bucket version; re-ingest required)")
	}
	return s.bucket.NewReader(ctx, path, nil)
}

func (s *BucketBlobStore) Delete(ctx context.Context, path string) error {
	if !strings.HasPrefix(path, bucketPrefix) {
		return nil // disk-era row; nothing of ours to delete
	}
	if err := s.bucket.Delete(ctx, path); err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		return fmt.Errorf("blob delete: %w", err)
	}
	return nil
}

func (s *BucketBlobStore) staticPrefix(moduleID, version string) string {
	return bucketPrefix + moduleID + "/" + version + "/static/"
}

func (s *BucketBlobStore) PutStatic(ctx context.Context, moduleID, version, relPath string, data []byte) error {
	key := s.staticPrefix(moduleID, version) + relPath
	if err := s.bucket.WriteAll(ctx, key, data, nil); err != nil {
		return fmt.Errorf("static put: %w", err)
	}
	return nil
}

func (s *BucketBlobStore) GetStatic(ctx context.Context, moduleID, version, relPath string) (io.ReadCloser, error) {
	return s.bucket.NewReader(ctx, s.staticPrefix(moduleID, version)+relPath, nil)
}

func (s *BucketBlobStore) DeleteStatic(ctx context.Context, moduleID, version string) error {
	it := s.bucket.List(&blob.ListOptions{Prefix: s.staticPrefix(moduleID, version)})
	for {
		obj, err := it.Next(ctx)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("static delete: list: %w", err)
		}
		if err := s.bucket.Delete(ctx, obj.Key); err != nil && gcerrors.Code(err) != gcerrors.NotFound {
			return fmt.Errorf("static delete: %w", err)
		}
	}
}
