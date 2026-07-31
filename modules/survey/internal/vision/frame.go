package vision

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
)

// Frame is one stdout line from the detector child. Empty Detections lines
// double as heartbeats.
type Frame struct {
	T          float64    `json:"t"` // unix seconds
	W          int        `json:"w"` // frame width in pixels
	Detections []FrameDet `json:"detections"`
}

type FrameDet struct {
	ID      int           `json:"id"`
	Corners [4][2]float64 `json:"corners"`
}

func DecodeFrame(line []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(line, &f); err != nil {
		return f, fmt.Errorf("vision: frame decode: %w", err)
	}
	if f.W <= 0 {
		return f, fmt.Errorf("vision: frame width %d", f.W)
	}
	return f, nil
}

// Reduce converts a decoded frame into rover-frame detections. The focal
// length is recomputed from the reported width so a camera that negotiated
// a different resolution still yields correct bearings.
func (f Frame) Reduce(hfovDeg, markerSizeM float64) []mission.Detection {
	if len(f.Detections) == 0 {
		return nil
	}
	focal := FocalPx(f.W, hfovDeg)
	t := time.Unix(0, int64(f.T*1e9))
	out := make([]mission.Detection, 0, len(f.Detections))
	for _, d := range f.Detections {
		out = append(out, FromCorners(d.ID, d.Corners, f.W, focal, markerSizeM, t))
	}
	return out
}
