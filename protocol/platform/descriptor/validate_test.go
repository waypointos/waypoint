package descriptor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(s string) string
		wantErr string
	}{
		{"missing schema", func(s string) string {
			return strings.Replace(s, "schema = 1", "", 1)
		}, "schema"},
		{"unsupported schema", func(s string) string {
			return strings.Replace(s, "schema = 1", "schema = 2", 1)
		}, "unsupported schema"},
		{"bad vehicle class", func(s string) string {
			return strings.Replace(s, `vehicle_class = "diff_drive_rover"`, `vehicle_class = "submarine"`, 1)
		}, "vehicle_class"},
		{"missing platform id", func(s string) string {
			return strings.Replace(s, `id = "test-rover"`, `id = ""`, 1)
		}, "platform.id"},
		{"bad driver kind", func(s string) string {
			return strings.Replace(s, `kind = "sim"`, `kind = "canbus"`, 1)
		}, "driver"},
		{"sts3215 requires port", func(s string) string {
			return strings.Replace(s, `kind = "sim"`, `kind = "sts3215"`, 1)
		}, "port"},
		{"duplicate joint name", func(s string) string {
			return strings.Replace(s, `name = "wheel_front_right"`, `name = "wheel_front_left"`, 1)
		}, "duplicate joint"},
		{"duplicate bus id", func(s string) string {
			return strings.Replace(s, "bus_id = 9", "bus_id = 10", 1)
		}, "duplicate bus_id"},
		{"unknown driver ref", func(s string) string {
			return strings.Replace(s, `driver = "main"`, `driver = "ghost"`, 1)
		}, "undefined driver"},
		{"bad joint type", func(s string) string {
			return strings.Replace(s, `type = "wheel"`, `type = "linear"`, 1)
		}, "joint type"},
		{"bad ownership", func(s string) string {
			return strings.Replace(s, `ownership = "platform"`, `ownership = "shared"`, 1)
		}, "ownership"},
		{"bad command interface", func(s string) string {
			return strings.Replace(s, `command_interfaces = ["velocity"]`, `command_interfaces = ["torque"]`, 1)
		}, "command_interfaces"},
		{"bad state interface", func(s string) string {
			return strings.Replace(s, `state_interfaces = ["position", "velocity"]`, `state_interfaces = ["position", "rpm"]`, 1)
		}, "state_interfaces"},
		{"platform wheel missing velocity limit", func(s string) string {
			return strings.Replace(s, "[joints.limits]\nvelocity_radps = 10.0", "", 1)
		}, "velocity_radps"},
		{"kinematics model mismatch", func(s string) string {
			return strings.Replace(s, `model = "diff_drive"`, `model = "ackermann"`, 1)
		}, "kinematics.model"},
		{"wheels ref unknown joint", func(s string) string {
			return strings.Replace(s, `front_left = "wheel_front_left"`, `front_left = "ghost_wheel"`, 1)
		}, "kinematics.wheels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.mutate(minimalValid)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidationPlatformRevoluteNeedsRange(t *testing.T) {
	src := minimalValid + `
[[joints]]
name = "mast"
driver = "main"
bus_id = 11
type = "revolute"
ownership = "platform"
command_interfaces = ["position"]
state_interfaces = ["position"]
`
	_, err := Parse([]byte(src))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "position_min_rad")
}

func TestValidationModuleOwnedJointMayOmitLimits(t *testing.T) {
	src := minimalValid + `
[[joints]]
name = "arm_1"
driver = "main"
bus_id = 1
type = "revolute"
ownership = "module"
command_interfaces = ["position"]
state_interfaces = ["position"]
`
	_, err := Parse([]byte(src))
	require.NoError(t, err)
}

func TestValidationBadSensorKind(t *testing.T) {
	src := minimalValid + `
[[sensors]]
name = "sonar"
kind = "sonar"
frame = "base_link"
rate_hz = 10
`
	_, err := Parse([]byte(src))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sensor")
}

func TestFixedBaseAcceptedWithoutKinematics(t *testing.T) {
	d, err := Parse([]byte(`
schema = 1
[platform]
id = "bench"
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
`))
	require.NoError(t, err)
	assert.Equal(t, "fixed_base", d.Platform.VehicleClass)
	assert.Nil(t, d.Kinematics)
}

func TestFixedBaseRejectsKinematics(t *testing.T) {
	_, err := Parse([]byte(`
schema = 1
[platform]
id = "bench"
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
[kinematics]
model = "diff_drive"
wheel_radius_m = 0.07
track_width_m = 0.3
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fixed_base must not declare [kinematics]")
}
