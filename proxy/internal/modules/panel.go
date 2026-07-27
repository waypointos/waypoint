package modules

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"

	"github.com/CalebQ42/squashfs"
)

// staticRoot is the directory inside a module image that holds its browser-
// served UI files. The agent serves the same directory for /module/<id>/static/*
// (gateway.ModuleStaticHandler); keep the two in sync.
const staticRoot = "dashboard"

// extractFile reads one file out of an in-memory squashfs image (the cosign-
// verified module .raw). pathInImage is the manifest bundle path (e.g.
// "/dashboard/panel.js"); squashfs paths are root-relative so the leading
// slash is stripped before the lookup.
func extractFile(raw []byte, pathInImage string) ([]byte, error) {
	rdr, err := squashfs.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open squashfs: %w", err)
	}
	return fs.ReadFile(rdr, strings.TrimPrefix(pathInImage, "/"))
}

// extractStaticTree reads every file under the image's static root, keyed by
// slash path relative to that root (e.g. "panel.js", "models/x/mesh.stl").
// An image without a static root yields an empty map: not every module ships
// a browser UI.
func extractStaticTree(raw []byte) (map[string][]byte, error) {
	rdr, err := squashfs.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open squashfs: %w", err)
	}
	if _, err := fs.Stat(rdr, staticRoot); err != nil {
		return map[string][]byte{}, nil
	}
	tree := map[string][]byte{}
	err = fs.WalkDir(rdr, staticRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(rdr, p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		tree[strings.TrimPrefix(p, staticRoot+"/")] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", staticRoot, err)
	}
	return tree, nil
}
