package modules

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractPanel_Fixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/umr-0.2.0.raw")
	require.NoError(t, err)
	got, err := extractFile(raw, "/dashboard/panel.js")
	require.NoError(t, err)
	require.Greater(t, len(got), 1000)
}

func TestExtractStaticTree_Fixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/tree-0.1.0.raw")
	require.NoError(t, err)
	tree, err := extractStaticTree(raw)
	require.NoError(t, err)
	// Nested files keyed relative to dashboard/; files outside it excluded.
	require.Len(t, tree, 3)
	require.Contains(t, tree, "panel.js")
	require.Contains(t, tree, "teleop.js")
	require.Contains(t, tree, "models/arm/mesh.stl")
	require.Equal(t, "solid arm\nendsolid arm\n", string(tree["models/arm/mesh.stl"]))
}

func TestExtractStaticTree_NoStaticRoot(t *testing.T) {
	// A module without a dashboard/ dir ships no browser UI: empty, not error.
	raw, err := os.ReadFile("testdata/noui-0.1.0.raw")
	require.NoError(t, err)
	tree, err := extractStaticTree(raw)
	require.NoError(t, err)
	require.Empty(t, tree)

	_, err = extractStaticTree([]byte("not a squashfs"))
	require.Error(t, err)
}
