package mission

import (
	"fmt"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

// Waypoint progress states reported in the mission view.
const (
	WPPending  = "pending"
	WPReached  = "reached"
	WPDetected = "detected"
)

// WaypointView is one waypoint's mission-progress row.
type WaypointView struct {
	I          int
	X, Y       float64
	TagID      int
	Status     string
	DetectedID int // -1 until a sense dwell confirms an id
}

// DetectionMark records the last id confirmed by a sense dwell.
type DetectionMark struct {
	WP int
	ID int
	T  time.Time
}

// View is a point-in-time snapshot for the UI mission doc: read-only, no
// engine internals leak out.
type View struct {
	State     string
	Mode      string
	Leg       int
	Waypoints []WaypointView
	Planned   [][2]float64
	Pose      nav.Pose
	Start     nav.Pose
	LastDet   *DetectionMark
	StartedAt time.Time // zero until the mission first arms
}

func (e *Engine) View() View {
	e.mu.Lock()
	defer e.mu.Unlock()
	wps := make([]WaypointView, len(e.p.Waypoints))
	for i, wp := range e.p.Waypoints {
		v := WaypointView{I: i, X: wp.X, Y: wp.Y, TagID: TagAny, Status: WPPending, DetectedID: -1}
		if i < len(e.p.TagIDs) {
			v.TagID = e.p.TagIDs[i]
		}
		switch {
		case i < len(e.wpDetected) && e.wpDetected[i] >= 0:
			v.Status, v.DetectedID = WPDetected, e.wpDetected[i]
		case i < len(e.wpReached) && e.wpReached[i]:
			v.Status = WPReached
		}
		wps[i] = v
	}
	var det *DetectionMark
	if e.lastDet != nil {
		d := *e.lastDet
		det = &d
	}
	return View{
		State:     e.stateName(),
		Mode:      modeName(e.mode),
		Leg:       e.li,
		Waypoints: wps,
		Planned:   buildPlanned(e.p),
		Pose:      e.pose,
		Start:     e.p.Start,
		LastDet:   det,
		StartedAt: e.startedAt,
	}
}

// SetPoseListener registers fn to run after every odometry update with the
// new believed pose. Invoked outside the engine lock.
func (e *Engine) SetPoseListener(fn func(nav.Pose)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onPose = fn
}

// SetSenseListener registers fn for the first accepted detection of each
// sense dwell. Invoked outside the engine lock.
func (e *Engine) SetSenseListener(fn func(wp, id int)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onSense = fn
}

// Reload swaps the mission waypoint set. Only legal between missions (IDLE
// or DONE); a DONE engine drops back to IDLE so the next autonomous arm
// runs the new set. Empty tags means accept-any on every waypoint.
func (e *Engine) Reload(wps []nav.Vec, tags []int, start nav.Pose, now time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != StateIdle && e.state != StateDone {
		return fmt.Errorf("mission is %s: waypoints can only change while IDLE or DONE", e.stateName())
	}
	if len(tags) == 0 {
		tags = make([]int, len(wps))
		for i := range tags {
			tags[i] = TagAny
		}
	}
	if len(tags) != len(wps) {
		return fmt.Errorf("%d tag ids for %d waypoints", len(tags), len(wps))
	}
	e.p.Waypoints, e.p.TagIDs, e.p.Start = wps, tags, start
	e.pose = start
	e.legs, e.li = nil, 0
	e.wpReached, e.wpDetected, e.lastDet = nil, nil, nil
	e.setState(StateIdle, now)
	e.event(now, "waypoints_reload", fmt.Sprintf("%d waypoints", len(wps)))
	return nil
}

// buildPlanned is the full planned polyline: start, outbound waypoints, and
// the reverse path home when return_home is set.
func buildPlanned(p Params) [][2]float64 {
	out := make([][2]float64, 0, 2*len(p.Waypoints)+2)
	out = append(out, [2]float64{p.Start.X, p.Start.Y})
	for _, wp := range p.Waypoints {
		out = append(out, [2]float64{wp.X, wp.Y})
	}
	if p.ReturnHome && len(p.Waypoints) > 0 {
		for i := len(p.Waypoints) - 2; i >= 0; i-- {
			out = append(out, [2]float64{p.Waypoints[i].X, p.Waypoints[i].Y})
		}
		out = append(out, [2]float64{p.Start.X, p.Start.Y})
	}
	return out
}

func modeName(m Mode) string {
	switch m {
	case ModeManual:
		return "MANUAL"
	case ModeSafe:
		return "SAFE"
	case ModeAutonomous:
		return "AUTONOMOUS"
	case ModeEstop:
		return "ESTOP"
	}
	return "UNKNOWN"
}
