package modules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderHardwareDropin_Empty(t *testing.T) {
	got := RenderHardwareDropin(Hardware{}, nil)
	require.Equal(t, "", got)
}

func TestRenderHardwareDropin_DevicesAndPaths(t *testing.T) {
	got := RenderHardwareDropin(Hardware{
		Devices:      []string{"/dev/i2c-1", "/dev/spidev0.0"},
		ReadOnly:     []string{"/sys/class/hwmon"},
		ReadWrite:    []string{"/var/lib/power-monitor"},
		Capabilities: []string{"NET_ADMIN"},
	}, nil)
	require.Equal(t,
		`[Service]
DeviceAllow=/dev/i2c-1 rw
DeviceAllow=/dev/spidev0.0 rw
BindReadOnlyPaths=/sys/class/hwmon
BindPaths=/var/lib/power-monitor
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN
`, got)
}

func TestRenderHardwareDropin_ReadWriteBackedBySource(t *testing.T) {
	src := func(p string) string { return "/data/waypoint/module-state" + p }
	got := RenderHardwareDropin(Hardware{
		ReadWrite: []string{"/var/lib/waypoint-module-so100"},
	}, src)
	require.Equal(t,
		`[Service]
BindPaths=/data/waypoint/module-state/var/lib/waypoint-module-so100:/var/lib/waypoint-module-so100
`, got)
}

func TestRenderComponentDropin(t *testing.T) {
	got := RenderComponentDropin([]Component{{Class: "arm", StateRateHz: 20}})
	want := "[Service]\n" +
		"Environment=WAYPOINT_MODULE_COMPONENT=arm\n" +
		"Environment=WAYPOINT_MODULE_STATE_RATE_HZ=20\n" +
		"Environment=WAYPOINT_MODULE_STATE_RATE_HZ_ARM=20\n"
	assert.Equal(t, want, got)
	assert.Equal(t, "", RenderComponentDropin(nil))
}

func TestRenderComponentDropinMulti(t *testing.T) {
	got := RenderComponentDropin([]Component{
		{Class: "drill", StateRateHz: 20},
		{Class: "soil-probe", StateRateHz: 2.5},
	})
	want := "[Service]\n" +
		"Environment=WAYPOINT_MODULE_COMPONENT=drill\n" +
		"Environment=WAYPOINT_MODULE_STATE_RATE_HZ=20\n" +
		"Environment=WAYPOINT_MODULE_STATE_RATE_HZ_DRILL=20\n" +
		"Environment=WAYPOINT_MODULE_STATE_RATE_HZ_SOIL_PROBE=2.5\n"
	assert.Equal(t, want, got)
}
