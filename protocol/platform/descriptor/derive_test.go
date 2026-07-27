package descriptor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T) *Descriptor {
	t.Helper()
	d, err := Parse([]byte(minimalValid))
	require.NoError(t, err)
	return d
}

func TestBusIDMaps(t *testing.T) {
	d := mustParse(t)
	id, ok := d.BusIDFor("wheel_front_left")
	require.True(t, ok)
	assert.Equal(t, uint32(10), id)
	name, ok := d.NameForBusID("main", 7)
	require.True(t, ok)
	assert.Equal(t, "wheel_back_left", name)
	_, ok = d.BusIDFor("ghost")
	assert.False(t, ok)
}

func TestPlatformOwnedBusIDs(t *testing.T) {
	d := mustParse(t)
	assert.ElementsMatch(t, []uint32{7, 8, 9, 10}, d.PlatformOwnedBusIDs())
}

func TestWheelSpeedsStraight(t *testing.T) {
	d := mustParse(t)
	w, err := d.WheelSpeeds(1.0, 0.0)
	require.NoError(t, err)
	// 1.0 / 0.07425 = 13.468013468013468; left side inverted.
	assert.InDelta(t, -13.468013468013468, w["wheel_front_left"], 1e-9)
	assert.InDelta(t, -13.468013468013468, w["wheel_back_left"], 1e-9)
	assert.InDelta(t, 13.468013468013468, w["wheel_front_right"], 1e-9)
	assert.InDelta(t, 13.468013468013468, w["wheel_back_right"], 1e-9)
}

func TestWheelSpeedsPureYaw(t *testing.T) {
	d := mustParse(t)
	w, err := d.WheelSpeeds(0.0, 1.0)
	require.NoError(t, err)
	// vL = -0.15, vR = +0.15; left side inverted makes all four positive.
	assert.InDelta(t, 2.0202020202020203, w["wheel_front_left"], 1e-9)
	assert.InDelta(t, 2.0202020202020203, w["wheel_back_left"], 1e-9)
	assert.InDelta(t, 2.0202020202020203, w["wheel_front_right"], 1e-9)
	assert.InDelta(t, 2.0202020202020203, w["wheel_back_right"], 1e-9)
}
