// Package vision turns ArUco detections into rover-frame geometry. The
// detector itself is a Python child process (or a synthetic source in sim
// mode); this package owns the pixel-to-bearing/distance math.
package vision

import (
	"math"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
)

// FocalPx derives the pinhole focal length in pixels from the horizontal
// field of view and frame width.
func FocalPx(widthPx int, hfovDeg float64) float64 {
	return float64(widthPx) / (2 * math.Tan(hfovDeg*math.Pi/360))
}

// FromCorners reduces one marker's corner quad to bearing and distance.
// Bearing is positive when the marker is left of frame center, matching the
// CCW-positive yaw convention. Distance uses the apparent side length of a
// square marker of known physical size.
func FromCorners(id int, corners [4][2]float64, frameW int, focalPx, markerSizeM float64, t time.Time) mission.Detection {
	var cx float64
	var side float64
	for i := 0; i < 4; i++ {
		cx += corners[i][0]
		j := (i + 1) % 4
		side += math.Hypot(corners[j][0]-corners[i][0], corners[j][1]-corners[i][1])
	}
	cx /= 4
	side /= 4

	det := mission.Detection{
		ID:      id,
		Bearing: math.Atan2(float64(frameW)/2-cx, focalPx),
		T:       t,
	}
	if side > 1 {
		det.Distance = markerSizeM * focalPx / side
	}
	return det
}
