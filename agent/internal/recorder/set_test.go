package recorder

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/waypointos/waypoint/protocol/platform/descriptor"
)

func roverDesc(t *testing.T) *descriptor.Descriptor {
	d, err := descriptor.Load("../../../protocol/platform/waypoint-rover.toml")
	require.NoError(t, err)
	return d
}

func benchDesc(t *testing.T) *descriptor.Descriptor {
	d, err := descriptor.Load("../../../protocol/platform/waypoint-bench.toml")
	require.NoError(t, err)
	return d
}

func subjects(specs []StreamSpec) []string {
	var out []string
	for _, s := range specs {
		out = append(out, s.Subject)
	}
	return out
}

func TestResolveSetRover(t *testing.T) {
	specs := ResolveSet(roverDesc(t), nil)
	subj := subjects(specs)
	require.Contains(t, subj, "telemetry.drive")
	require.Contains(t, subj, "telemetry.motors")
	require.Contains(t, subj, "telemetry.power")
	require.Contains(t, subj, "cmd.drive") // body_twist altitude
	require.Contains(t, subj, "cmd.servo") // joint_position altitude
}

func TestResolveSetBenchWithArmComponent(t *testing.T) {
	specs := ResolveSet(benchDesc(t), map[string][]string{"so100": {"arm"}})
	subj := subjects(specs)
	require.NotContains(t, subj, "telemetry.drive") // bench has no drive
	require.NotContains(t, subj, "cmd.drive")       // no body_twist altitude
	require.Contains(t, subj, "cmd.servo")
	require.Contains(t, subj, "module.so100.arm.state")
	require.Contains(t, subj, "module.so100.arm.cmd")
}

func TestResolveSetNilDescriptor(t *testing.T) {
	specs := ResolveSet(nil, map[string][]string{"env": {"sensor"}})
	require.Equal(t,
		[]StreamSpec{{Subject: "module.env.sensor.state", Message: "waypoint.v1.SensorReadings"}},
		specs)
}

func TestResolveSetGenericClass(t *testing.T) {
	specs := ResolveSet(nil, map[string][]string{"mydrill": {"drill"}})
	require.Equal(t, []StreamSpec{
		{Subject: "module.mydrill.drill.state"},
		{Subject: "module.mydrill.drill.cmd"},
	}, specs)
}

func TestResolveSetSkipsEmptyClass(t *testing.T) {
	require.Empty(t, ResolveSet(nil, map[string][]string{"mydrill": {""}}))
}

func TestResolveSetMultiComponentModule(t *testing.T) {
	specs := ResolveSet(nil, map[string][]string{"drill": {"drill", "sensor"}})
	require.Equal(t, []StreamSpec{
		{Subject: "module.drill.drill.state"},
		{Subject: "module.drill.drill.cmd"},
		{Subject: "module.drill.sensor.state", Message: "waypoint.v1.SensorReadings"},
	}, specs)
}

func TestResolveSetDeduplicates(t *testing.T) {
	specs := ResolveSet(roverDesc(t), nil)
	seen := map[string]bool{}
	for _, s := range specs {
		require.False(t, seen[s.Subject], "duplicate %s", s.Subject)
		seen[s.Subject] = true
	}
}
