package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

func TestResolveDefaults(t *testing.T) {
	c, err := Resolve(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 0.35, c.CruiseSpeedMps)
	assert.Equal(t, 0.8, c.TurnRateRadps)
	assert.Equal(t, 0.45, c.ArriveRadiusM)
	assert.Equal(t, 0.55, c.TagStopM)
	assert.Equal(t, 4.0, c.SenseDwellS)
	assert.Equal(t, 120.0, c.ScanMaxDeg)
	assert.True(t, c.ReturnHome)
	assert.Equal(t, "/dev/video0", c.CameraDevice)
	assert.Equal(t, 1280, c.CameraWidth)
	assert.Equal(t, 8, c.LEDCount)
	assert.Equal(t, 0.4, c.LEDBrightness)
	assert.False(t, c.SimMode)
	assert.Empty(t, c.Waypoints)
}

func TestResolveFull(t *testing.T) {
	c, err := Resolve(map[string]any{
		"waypoints":        "1,2; 3.5,-4 ;0,0",
		"tag_ids":          "17, 40, 99",
		"start_pose":       "1,1,90",
		"cruise_speed_mps": 0.5,
		"return_home":      false,
		"sim_mode":         true,
		"led_count":        int64(12),
	})
	require.NoError(t, err)
	assert.Equal(t, []nav.Vec{{X: 1, Y: 2}, {X: 3.5, Y: -4}, {X: 0, Y: 0}}, c.Waypoints)
	assert.Equal(t, []int{17, 40, 99}, c.TagIDs)
	assert.InDelta(t, math.Pi/2, c.Start.Theta, 1e-9)
	assert.Equal(t, 1.0, c.Start.X)
	assert.Equal(t, 0.5, c.CruiseSpeedMps)
	assert.False(t, c.ReturnHome)
	assert.True(t, c.SimMode)
	assert.Equal(t, 12, c.LEDCount)
}

func TestResolveCoercesStrings(t *testing.T) {
	// The dashboard form may serialize numbers and bools as strings.
	c, err := Resolve(map[string]any{
		"cruise_speed_mps": "0.25",
		"led_count":        "6",
		"sim_mode":         "true",
	})
	require.NoError(t, err)
	assert.Equal(t, 0.25, c.CruiseSpeedMps)
	assert.Equal(t, 6, c.LEDCount)
	assert.True(t, c.SimMode)
}

func TestResolveEmptyTagIDsAcceptAny(t *testing.T) {
	c, err := Resolve(map[string]any{"waypoints": "1,0;2,0"})
	require.NoError(t, err)
	assert.Equal(t, []int{-1, -1}, c.TagIDs)
}

func TestResolveTagCountMismatch(t *testing.T) {
	_, err := Resolve(map[string]any{"waypoints": "1,0;2,0", "tag_ids": "5"})
	assert.Error(t, err)
}

func TestResolveTooManyWaypoints(t *testing.T) {
	s := ""
	for i := 0; i < 11; i++ {
		s += "1,1;"
	}
	_, err := Resolve(map[string]any{"waypoints": s})
	assert.Error(t, err)
}

func TestResolveBadWaypoint(t *testing.T) {
	_, err := Resolve(map[string]any{"waypoints": "1,2;three,4"})
	assert.Error(t, err)
}

func TestResolveBadBrightness(t *testing.T) {
	_, err := Resolve(map[string]any{"led_brightness": 1.5})
	assert.Error(t, err)
}

func TestSimTagFor(t *testing.T) {
	c, err := Resolve(map[string]any{
		"waypoints": "1,0;2,0;3,0",
		"tag_ids":   "7,8,9",
		"sim_tags":  "50",
	})
	require.NoError(t, err)
	assert.Equal(t, 50, c.SimTagFor(0)) // explicit sim tag wins
	assert.Equal(t, 8, c.SimTagFor(1))  // falls back to expected tag
	c.TagIDs[2] = -1
	assert.Equal(t, 3, c.SimTagFor(2)) // stable fallback
}

func TestParseStartPose(t *testing.T) {
	_, err := ParseStartPose("1,2")
	assert.Error(t, err)
	p, err := ParseStartPose("0,0,180")
	assert.NoError(t, err)
	assert.InDelta(t, math.Pi, p.Theta, 1e-9)
}
