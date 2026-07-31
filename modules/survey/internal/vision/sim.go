package vision

import (
	"context"
	"math"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

// Sim synthesizes detections from a pose source: whenever the pose is
// within Visibility of a waypoint and the marker falls inside the camera
// FOV, it emits a detection with the true bearing and distance.
type Sim struct {
	Waypoints  []nav.Vec
	Tags       []int // marker id faked per waypoint
	Visibility float64
	HfovDeg    float64
	Pose       func() nav.Pose
}

// Detect returns the synthetic detections for the current pose.
func (s *Sim) Detect(now time.Time) []mission.Detection {
	p := s.Pose()
	half := s.HfovDeg * math.Pi / 360
	var out []mission.Detection
	for i, wp := range s.Waypoints {
		d := nav.Dist(p.Position(), wp)
		if d > s.Visibility || d < 1e-6 {
			continue
		}
		b := nav.BearingTo(p, wp)
		if math.Abs(b) > half {
			continue
		}
		out = append(out, mission.Detection{ID: s.Tags[i], Bearing: b, Distance: d, T: now})
	}
	return out
}

// Run emits detections at ~10 Hz until ctx is done.
func (s *Sim) Run(ctx context.Context, h Handler) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if dets := s.Detect(now); len(dets) > 0 {
				h(dets, now)
			}
		}
	}
}
