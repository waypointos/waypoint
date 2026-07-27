package descriptor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalValid = `
schema = 1

[platform]
id = "test-rover"
name = "Test Rover"
vehicle_class = "diff_drive_rover"

[drivers.main]
kind = "sim"

[[joints]]
name = "wheel_front_left"
driver = "main"
bus_id = 10
type = "wheel"
ownership = "platform"
invert = true
command_interfaces = ["velocity"]
state_interfaces = ["position", "velocity"]
[joints.limits]
velocity_radps = 10.0

[[joints]]
name = "wheel_front_right"
driver = "main"
bus_id = 9
type = "wheel"
ownership = "platform"
command_interfaces = ["velocity"]
state_interfaces = ["position", "velocity"]
[joints.limits]
velocity_radps = 10.0

[[joints]]
name = "wheel_back_left"
driver = "main"
bus_id = 7
type = "wheel"
ownership = "platform"
invert = true
command_interfaces = ["velocity"]
state_interfaces = ["position", "velocity"]
[joints.limits]
velocity_radps = 10.0

[[joints]]
name = "wheel_back_right"
driver = "main"
bus_id = 8
type = "wheel"
ownership = "platform"
command_interfaces = ["velocity"]
state_interfaces = ["position", "velocity"]
[joints.limits]
velocity_radps = 10.0

[kinematics]
model = "diff_drive"
wheel_radius_m = 0.07425
track_width_m = 0.30
wheels = { front_left = "wheel_front_left", front_right = "wheel_front_right", back_left = "wheel_back_left", back_right = "wheel_back_right" }
`

func TestParseMinimalValid(t *testing.T) {
	d, err := Parse([]byte(minimalValid))
	require.NoError(t, err)
	assert.Equal(t, 1, d.Schema)
	assert.Equal(t, "test-rover", d.Platform.ID)
	assert.Equal(t, "diff_drive_rover", d.Platform.VehicleClass)
	assert.Len(t, d.Joints, 4)
	assert.Equal(t, "sim", d.Drivers["main"].Kind)
	require.NotNil(t, d.Kinematics)
	assert.Equal(t, 0.30, d.Kinematics.TrackWidthM)
}

func TestParseRejectsUnknownKey(t *testing.T) {
	bad := minimalValid + "\n[typo_section]\nfoo = 1\n"
	_, err := Parse([]byte(bad))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown key")
}
