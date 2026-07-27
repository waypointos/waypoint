package modules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDenyIDsFromDescriptor(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("../../../protocol/platform/waypoint-rover.toml")
	require.NoError(t, err)
	path := filepath.Join(dir, "platform.toml")
	require.NoError(t, os.WriteFile(path, src, 0o644))

	deny := loadDenyIDs([]string{path})
	assert.Equal(t, map[uint32]struct{}{7: {}, 8: {}, 9: {}, 10: {}}, deny)
}

func TestLoadDenyIDsFallsBackWhenMissing(t *testing.T) {
	deny := loadDenyIDs([]string{filepath.Join(t.TempDir(), "absent.toml")})
	// Old-image fallback: the previous hardcoded drive-wheel set.
	assert.Equal(t, map[uint32]struct{}{7: {}, 8: {}, 9: {}, 10: {}}, deny)
}

func TestLoadDenyIDsBenchDescriptorYieldsEmptyDenyList(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "platform.toml")
	require.NoError(t, os.WriteFile(p, []byte(`
schema = 1
[platform]
id = "waypoint-bench"
vehicle_class = "fixed_base"
[drivers.main]
kind = "sts3215"
port = "/dev/ttyAMA0"
[[joints]]
name = "arm_1"
driver = "main"
bus_id = 1
type = "revolute"
ownership = "module"
command_interfaces = ["position"]
state_interfaces = ["position"]
`), 0o644))
	deny := loadDenyIDs([]string{"", p})
	assert.Empty(t, deny, "fixed_base with all-module joints must produce an empty deny-list, not the hardcoded fallback")
}
