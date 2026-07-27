// Package basemap is the rover's read-through map-tile cache: it serves
// /basemap/{z}/{x}/{y}.mvt from disk, filling misses from the proxy over NATS
// and serving cached tiles when the link is down.
package basemap

import (
	"fmt"
	"os"
	"path/filepath"
)

// Cache stores tiles under dir as {z}/{x}/{y}.mvt.
type Cache struct{ dir string }

func NewCache(dir string) *Cache { return &Cache{dir: dir} }

func (c *Cache) path(z, x, y uint32) string {
	return filepath.Join(c.dir, fmt.Sprint(z), fmt.Sprint(x), fmt.Sprintf("%d.mvt", y))
}

func (c *Cache) Load(z, x, y uint32) ([]byte, bool) {
	b, err := os.ReadFile(c.path(z, x, y))
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *Cache) Store(z, x, y uint32, data []byte) error {
	p := c.path(z, x, y)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, p) // atomic publish
}
