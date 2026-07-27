package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BlobStore stores .raw artifacts and the static UI tree extracted from them
// (the image's dashboard/ directory, served to browsers file by file).
type BlobStore interface {
	Put(ctx context.Context, moduleID, version string, src io.Reader) (string, error)
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	// Delete removes a stored artifact by the path Put returned. A missing
	// blob is not an error.
	Delete(ctx context.Context, path string) error
	// relPath is a slash-separated path under the image's static root,
	// already sanitized by the caller (no "..", no leading slash).
	PutStatic(ctx context.Context, moduleID, version, relPath string, data []byte) error
	GetStatic(ctx context.Context, moduleID, version, relPath string) (io.ReadCloser, error)
	// DeleteStatic removes a version's whole cached static tree. A missing
	// tree is not an error.
	DeleteStatic(ctx context.Context, moduleID, version string) error
}

type DiskBlobStore struct {
	root string
}

func NewDiskBlobStore(root string) *DiskBlobStore {
	return &DiskBlobStore{root: filepath.Clean(root)}
}

func (s *DiskBlobStore) Put(_ context.Context, moduleID, version string, src io.Reader) (string, error) {
	dir := filepath.Join(s.root, moduleID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("blob put: mkdir: %w", err)
	}
	dest := filepath.Join(dir, version+".raw")
	tmp, err := os.CreateTemp(dir, ".raw-tmp-*")
	if err != nil {
		return "", fmt.Errorf("blob put: temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("blob put: copy: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return dest, nil
}

func (s *DiskBlobStore) Get(_ context.Context, path string) (io.ReadCloser, error) {
	cleaned := filepath.Clean(path)
	if !strings.HasPrefix(cleaned, s.root+string(filepath.Separator)) {
		return nil, errors.New("blob get: path outside store root")
	}
	return os.Open(cleaned)
}

func (s *DiskBlobStore) Delete(_ context.Context, path string) error {
	cleaned := filepath.Clean(path)
	if !strings.HasPrefix(cleaned, s.root+string(filepath.Separator)) {
		return errors.New("blob delete: path outside store root")
	}
	if err := os.Remove(cleaned); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// staticDir is where a version's extracted static tree lives on disk. Module
// id, version, and relPath appear in request URLs, so every accessor checks
// the resolved path stays inside it.
func (s *DiskBlobStore) staticDir(moduleID, version string) string {
	return filepath.Join(s.root, moduleID, version+".static")
}

// PutStatic caches one file of a version's static tree using an atomic
// temp+rename so readers never see a partial write.
func (s *DiskBlobStore) PutStatic(_ context.Context, moduleID, version, relPath string, data []byte) error {
	base := s.staticDir(moduleID, version)
	dest := filepath.Join(base, filepath.FromSlash(relPath))
	if !strings.HasPrefix(dest, base+string(filepath.Separator)) {
		return errors.New("static put: path outside static dir")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("static put: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".static-tmp-*")
	if err != nil {
		return fmt.Errorf("static put: temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("static put: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func (s *DiskBlobStore) GetStatic(_ context.Context, moduleID, version, relPath string) (io.ReadCloser, error) {
	base := s.staticDir(moduleID, version)
	dest := filepath.Join(base, filepath.FromSlash(relPath))
	if !strings.HasPrefix(dest, base+string(filepath.Separator)) {
		return nil, errors.New("static get: path outside static dir")
	}
	return os.Open(dest)
}

func (s *DiskBlobStore) DeleteStatic(_ context.Context, moduleID, version string) error {
	base := s.staticDir(moduleID, version)
	if !strings.HasPrefix(base, s.root+string(filepath.Separator)) {
		return errors.New("static delete: path outside store root")
	}
	return os.RemoveAll(base)
}
