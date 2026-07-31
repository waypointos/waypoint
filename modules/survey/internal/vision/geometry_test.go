package vision

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFocalPx(t *testing.T) {
	// 90 degree HFOV: focal equals half the width.
	assert.InDelta(t, 640, FocalPx(1280, 90), 1e-9)
	// Narrower FOV means a longer focal.
	assert.Greater(t, FocalPx(1280, 66), 640.0)
}

func square(cx, cy, side float64) [4][2]float64 {
	h := side / 2
	return [4][2]float64{
		{cx - h, cy - h}, {cx + h, cy - h}, {cx + h, cy + h}, {cx - h, cy + h},
	}
}

func TestFromCornersCentered(t *testing.T) {
	focal := FocalPx(1280, 66)
	// 0.20 m marker at 2 m: apparent side = size * focal / dist.
	side := 0.20 * focal / 2.0
	det := FromCorners(17, square(640, 360, side), 1280, focal, 0.20, time.Unix(1, 0))
	assert.Equal(t, 17, det.ID)
	assert.InDelta(t, 0, det.Bearing, 1e-9)
	assert.InDelta(t, 2.0, det.Distance, 1e-6)
}

func TestFromCornersBearingSign(t *testing.T) {
	focal := FocalPx(1280, 66)
	// Marker left of center: positive bearing (CCW toward it).
	left := FromCorners(1, square(320, 360, 40), 1280, focal, 0.20, time.Time{})
	assert.Positive(t, left.Bearing)
	right := FromCorners(1, square(960, 360, 40), 1280, focal, 0.20, time.Time{})
	assert.Negative(t, right.Bearing)
	assert.InDelta(t, left.Bearing, -right.Bearing, 1e-9)
	// Sanity: 320 px off center at this focal is about 17.7 degrees.
	assert.InDelta(t, math.Atan2(320, focal), left.Bearing, 1e-9)
}

func TestFromCornersTinyMarkerNoDistance(t *testing.T) {
	det := FromCorners(1, square(640, 360, 0.5), 1280, 1000, 0.20, time.Time{})
	assert.Zero(t, det.Distance)
}

func TestDecodeFrame(t *testing.T) {
	line := []byte(`{"t":1700000000.5,"w":1280,"detections":[{"id":17,"corners":[[600,300],[680,300],[680,380],[600,380]]}]}`)
	f, err := DecodeFrame(line)
	require.NoError(t, err)
	assert.Equal(t, 1280, f.W)
	dets := f.Reduce(66, 0.20)
	require.Len(t, dets, 1)
	assert.Equal(t, 17, dets[0].ID)
	assert.InDelta(t, 0, dets[0].Bearing, 1e-6)
	assert.Positive(t, dets[0].Distance)
	assert.Equal(t, int64(1700000000), dets[0].T.Unix())
}

func TestDecodeFrameHeartbeat(t *testing.T) {
	f, err := DecodeFrame([]byte(`{"t":1,"w":1280,"detections":[]}`))
	require.NoError(t, err)
	assert.Empty(t, f.Reduce(66, 0.20))
}

func TestDecodeFrameBad(t *testing.T) {
	_, err := DecodeFrame([]byte(`{"t":1}`))
	assert.Error(t, err)
	_, err = DecodeFrame([]byte(`not json`))
	assert.Error(t, err)
}
