package basemap

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCache_StoreThenLoad(t *testing.T) {
	c := NewCache(t.TempDir())
	require.NoError(t, c.Store(3, 4, 5, []byte("tile")))

	got, ok := c.Load(3, 4, 5)
	require.True(t, ok)
	require.Equal(t, []byte("tile"), got)

	_, ok = c.Load(9, 9, 9)
	require.False(t, ok)
}

func TestCache_PathIsHierarchical(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir)
	require.NoError(t, c.Store(3, 4, 5, []byte("x")))
	require.FileExists(t, filepath.Join(dir, "3", "4", "5.mvt"))
}
