// Package mission is the pure mission state machine: it consumes mode,
// odometry, and ArUco detections and produces drive setpoints, an LED
// state, and log events. No I/O so every transition is unit-testable.
package mission

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

// Mode mirrors waypoint.v1.Mode; the engine only cares about AUTONOMOUS.
type Mode int32

const (
	ModeUnspecified Mode = 0
	ModeManual      Mode = 1
	ModeSafe        Mode = 2
	ModeAutonomous  Mode = 3
	ModeEstop       Mode = 4
)

type State int

const (
	StateIdle State = iota
	StateTransit
	StateScan
	StateSense
	StateDone
)

// Tag sentinel values for a leg's expected marker.
const (
	TagAny  = -1 // accept any marker id
	TagNone = -2 // pure odometry leg (the return-to-start leg)
)

const (
	modeFreshFor = 2500 * time.Millisecond
	detFreshFor  = 800 * time.Millisecond
	// Window checked at arrival to decide sense-directly vs scan.
	arriveTagWindow = 1200 * time.Millisecond
	// Rotate in place above this heading error instead of arcing.
	alignThreshold = 0.35
	yawKp          = 1.5
	// Approach speed tapers with distance so arrival does not overshoot.
	approachGain     = 0.8
	minApproachSpeed = 0.08
	scanRateScale    = 0.5
	// Heading is corrected against marker bearings only beyond this range:
	// closer in, position error dominates the world-bearing estimate.
	headingFixMinDist = 1.0
	headingFixGain    = 0.15
)

type Params struct {
	Waypoints    []nav.Vec
	TagIDs       []int // per waypoint; TagAny accepts any marker
	Start        nav.Pose
	CruiseSpeed  float64
	TurnRate     float64
	ArriveRadius float64
	TagStop      float64
	SenseDwell   time.Duration
	ScanMaxRad   float64
	ReturnHome   bool
}

// Detection is one ArUco sighting reduced to rover-frame geometry.
type Detection struct {
	ID       int
	Bearing  float64 // radians, positive = marker to the left
	Distance float64 // meters from apparent size; 0 = unknown
	T        time.Time
}

type Event struct {
	T      time.Time
	Kind   string
	Detail string
}

// Drive is the setpoint for one publish cycle. A nil Drive in Output means
// publish nothing (the platform failsafe zeroes motion after 300 ms).
type Drive struct {
	VX      float64
	YawRate float64
}

type LEDPhase int

const (
	LEDGreen LEDPhase = iota
	LEDRed
	LEDYellow
)

type LEDState struct {
	Phase  LEDPhase
	ID     int
	ShowID bool
}

type Output struct {
	Drive  *Drive
	LED    LEDState
	Events []Event
	State  string
}

// Snap is a point-in-time view for logging and the sim vision source.
type Snap struct {
	Pose    nav.Pose
	State   string
	SpeedVX float64 // last measured body vx
	FreshID int     // fresh detection id, -1 if none
}

type leg struct {
	target  nav.Vec
	tag     int
	wpIndex int // original waypoint index, -1 for the start point
	sense   bool
}

type Engine struct {
	mu  sync.Mutex
	p   Params
	cfg struct{ outbound int }

	pose      nav.Pose
	measVX    float64
	lastOdoAt time.Time

	mode       Mode
	lastModeAt time.Time
	paused     bool
	pausedAt   time.Time

	state State
	legs  []leg
	li    int

	dets []Detection

	senseStart  time.Time
	senseCounts map[int]int
	senseGeom   map[int]Detection

	scanDir      int
	scanAccum    float64
	scanLastHead float64

	lastDetectedID int // last id confirmed in a sense dwell, -1 if none
	events         []Event
}

func New(p Params) *Engine {
	return &Engine{
		p:              p,
		pose:           p.Start,
		state:          StateIdle,
		lastDetectedID: -1,
	}
}

func (e *Engine) OnMode(m Mode, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	prev := e.mode
	e.mode = m
	e.lastModeAt = now
	if m == prev {
		return
	}
	if m == ModeAutonomous && e.state == StateIdle {
		e.arm(now)
	}
}

func (e *Engine) OnOdometry(vx, yawRate float64, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.lastOdoAt.IsZero() {
		dt := now.Sub(e.lastOdoAt).Seconds()
		// Clamp so a telemetry gap cannot teleport the estimate.
		if dt > 0 && dt <= 0.1 {
			e.pose.Integrate(vx, yawRate, dt)
		}
	}
	e.measVX = vx
	e.lastOdoAt = now
}

func (e *Engine) OnDetections(dets []Detection, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dets = dets
	if e.state != StateSense || e.li >= len(e.legs) {
		return
	}
	tag := e.legs[e.li].tag
	for _, d := range dets {
		if !tagMatches(tag, d.ID) {
			continue
		}
		e.senseCounts[d.ID]++
		e.senseGeom[d.ID] = d
	}
}

// Tick evaluates the state machine and returns the outputs for one 20 Hz
// control cycle.
func (e *Engine) Tick(now time.Time) Output {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := Output{}
	authorized := e.mode == ModeAutonomous && !e.lastModeAt.IsZero() &&
		now.Sub(e.lastModeAt) <= modeFreshFor

	if !authorized {
		if e.state == StateDone {
			// Operator took the rover back: rearm on the next autonomous switch.
			e.setState(StateIdle, now)
			e.event(now, "mission_reset", "mode left autonomous after DONE")
		}
		if e.missionActive() && !e.paused {
			e.paused = true
			e.pausedAt = now
			e.event(now, "pause", "mode not autonomous or mode mirror stale")
		}
		out.LED = LEDState{Phase: LEDGreen}
		out.State = e.stateName()
		out.Events = e.drainEvents()
		return out
	}

	if e.paused {
		e.paused = false
		// Shift the dwell timer so paused time does not count against it.
		if e.state == StateSense {
			e.senseStart = e.senseStart.Add(now.Sub(e.pausedAt))
		}
		e.event(now, "resume", "mode autonomous again")
	}

	switch e.state {
	case StateTransit:
		out.Drive = e.tickTransit(now)
	case StateScan:
		out.Drive = e.tickScan(now)
	case StateSense:
		out.Drive = &Drive{} // hold position, keep the failsafe fed
		e.tickSense(now)
	case StateDone:
		out.Drive = &Drive{}
	}
	// LED reads the post-transition state so a tick that changes state
	// reports consistently.
	out.LED = e.ledState()
	out.State = e.stateName()
	out.Events = e.drainEvents()
	return out
}

func (e *Engine) Snapshot() Snap {
	e.mu.Lock()
	defer e.mu.Unlock()
	return Snap{
		Pose:    e.pose,
		State:   e.stateName(),
		SpeedVX: e.measVX,
		FreshID: e.snapFreshID(),
	}
}

func (e *Engine) snapFreshID() int {
	if len(e.dets) == 0 {
		return -1
	}
	return e.dets[len(e.dets)-1].ID
}

func (e *Engine) arm(now time.Time) {
	if len(e.p.Waypoints) == 0 {
		e.event(now, "mission_skip", "no waypoints configured")
		return
	}
	e.legs = buildLegs(e.p)
	e.cfg.outbound = len(e.p.Waypoints)
	e.li = 0
	e.setState(StateTransit, now)
	e.event(now, "mission_start", fmt.Sprintf("%d waypoints, return_home=%v", len(e.p.Waypoints), e.p.ReturnHome))
}

func (e *Engine) tickTransit(now time.Time) *Drive {
	lg := e.legs[e.li]
	det, hasDet := e.freshDet(now, lg.tag, detFreshFor)
	d := nav.Dist(e.pose.Position(), lg.target)

	// A ranged marker is ground truth: while it says we are still out, the
	// odometry radius alone must not declare arrival.
	arrived := d <= e.p.ArriveRadius
	if hasDet && det.Distance > 0 {
		arrived = det.Distance <= e.p.TagStop
	}
	if arrived {
		e.arrive(now, lg)
		return &Drive{}
	}

	// Visual servoing wins over odometry bearing once the marker is seen.
	herr := nav.BearingTo(e.pose, lg.target)
	if hasDet {
		herr = det.Bearing
		if det.Distance > headingFixMinDist {
			// At range, the marker's world bearing is well-conditioned:
			// bleed the believed heading toward it to shed yaw drift.
			wb := math.Atan2(lg.target.Y-e.pose.Y, lg.target.X-e.pose.X)
			e.pose.Theta = nav.WrapAngle(e.pose.Theta +
				headingFixGain*nav.WrapAngle(wb-det.Bearing-e.pose.Theta))
		}
	}
	return e.steer(herr, d)
}

func (e *Engine) steer(headingErr, dist float64) *Drive {
	if math.Abs(headingErr) > alignThreshold {
		return &Drive{YawRate: math.Copysign(e.p.TurnRate, headingErr)}
	}
	vx := math.Min(e.p.CruiseSpeed, math.Max(minApproachSpeed, dist*approachGain))
	yaw := clamp(yawKp*headingErr, -e.p.TurnRate, e.p.TurnRate)
	return &Drive{VX: vx, YawRate: yaw}
}

func (e *Engine) arrive(now time.Time, lg leg) {
	e.event(now, "arrived", fmt.Sprintf("leg=%d wp=%d x=%.2f y=%.2f", e.li, lg.wpIndex, e.pose.X, e.pose.Y))
	if lg.sense {
		if _, seen := e.freshDet(now, lg.tag, arriveTagWindow); seen {
			e.enterSense(now)
		} else {
			e.enterScan(now)
		}
		return
	}
	// Return leg: no dwell, but a visible marker still refreshes the fix.
	if det, ok := e.freshDet(now, lg.tag, arriveTagWindow); ok && det.Distance > 0 {
		e.refix(det, lg)
		e.event(now, "refix", fmt.Sprintf("wp=%d id=%d", lg.wpIndex, det.ID))
	}
	e.advance(now)
}

func (e *Engine) enterScan(now time.Time) {
	e.setState(StateScan, now)
	e.scanDir = 1
	e.scanAccum = 0
	e.scanLastHead = e.pose.Theta
	e.event(now, "scan_start", fmt.Sprintf("wp=%d", e.legs[e.li].wpIndex))
}

func (e *Engine) tickScan(now time.Time) *Drive {
	lg := e.legs[e.li]
	if _, ok := e.freshDet(now, lg.tag, detFreshFor); ok {
		e.enterSense(now)
		return &Drive{}
	}
	e.scanAccum += nav.WrapAngle(e.pose.Theta - e.scanLastHead)
	e.scanLastHead = e.pose.Theta
	switch {
	case e.scanDir > 0 && e.scanAccum >= e.p.ScanMaxRad:
		e.scanDir = -1
	case e.scanDir < 0 && e.scanAccum <= -e.p.ScanMaxRad:
		// Sweep exhausted: dwell anyway so the mission keeps moving.
		e.event(now, "scan_giveup", fmt.Sprintf("wp=%d", lg.wpIndex))
		e.enterSense(now)
		return &Drive{}
	}
	return &Drive{YawRate: float64(e.scanDir) * e.p.TurnRate * scanRateScale}
}

func (e *Engine) enterSense(now time.Time) {
	e.setState(StateSense, now)
	e.senseStart = now
	e.senseCounts = map[int]int{}
	e.senseGeom = map[int]Detection{}
	e.event(now, "sense_start", fmt.Sprintf("wp=%d", e.legs[e.li].wpIndex))
}

func (e *Engine) tickSense(now time.Time) {
	if now.Sub(e.senseStart) < e.p.SenseDwell {
		return
	}
	lg := e.legs[e.li]
	id, ok := e.senseBest()
	if ok {
		e.lastDetectedID = id
		e.event(now, "detected", fmt.Sprintf("wp=%d id=%d bin=%07b", lg.wpIndex, id, id&0x7f))
		if det := e.senseGeom[id]; det.Distance > 0 {
			e.refix(det, lg)
		} else {
			e.snapTo(lg)
		}
	} else {
		e.event(now, "no_detection", fmt.Sprintf("wp=%d", lg.wpIndex))
		e.snapTo(lg)
	}
	e.advance(now)
}

// senseBest returns the id seen most often during the dwell; ties break to
// the lowest id for determinism.
func (e *Engine) senseBest() (int, bool) {
	best, bestN := -1, 0
	for id, n := range e.senseCounts {
		if n > bestN || (n == bestN && best >= 0 && id < best) {
			best, bestN = id, n
		}
	}
	return best, best >= 0
}

// refix back-projects the position from the marker sighting: the marker
// sits at the waypoint, so bearing plus distance place the rover. Heading
// is left to odometry; at close range a heading fix would amplify position
// error instead of removing it.
func (e *Engine) refix(det Detection, lg leg) {
	dir := e.pose.Theta + det.Bearing
	e.pose.X = lg.target.X - det.Distance*math.Cos(dir)
	e.pose.Y = lg.target.Y - det.Distance*math.Sin(dir)
}

// snapTo pins the position to the waypoint's surveyed coordinates so
// odometry drift cannot compound across legs.
func (e *Engine) snapTo(lg leg) {
	e.pose.X = lg.target.X
	e.pose.Y = lg.target.Y
}

func (e *Engine) advance(now time.Time) {
	e.li++
	if e.li >= len(e.legs) {
		e.setState(StateDone, now)
		e.event(now, "mission_end", fmt.Sprintf("x=%.2f y=%.2f", e.pose.X, e.pose.Y))
		return
	}
	e.setState(StateTransit, now)
}

func (e *Engine) ledState() LEDState {
	switch e.state {
	case StateIdle:
		return LEDState{Phase: LEDGreen}
	case StateSense:
		if id, ok := e.senseBest(); ok {
			return LEDState{Phase: LEDYellow, ID: id, ShowID: true}
		}
		return LEDState{Phase: LEDYellow}
	default:
		return LEDState{Phase: LEDRed}
	}
}

func (e *Engine) freshDet(now time.Time, tag int, window time.Duration) (Detection, bool) {
	var best Detection
	found := false
	for _, d := range e.dets {
		if !tagMatches(tag, d.ID) || now.Sub(d.T) > window {
			continue
		}
		if !found || d.T.After(best.T) {
			best = d
			found = true
		}
	}
	return best, found
}

func tagMatches(tag, id int) bool {
	switch tag {
	case TagNone:
		return false
	case TagAny:
		return true
	default:
		return id == tag
	}
}

func (e *Engine) missionActive() bool {
	return e.state == StateTransit || e.state == StateScan || e.state == StateSense
}

func (e *Engine) setState(s State, now time.Time) {
	if s == e.state {
		return
	}
	e.state = s
	e.event(now, "state", e.stateName())
}

func (e *Engine) stateName() string {
	switch e.state {
	case StateIdle:
		return "IDLE"
	case StateTransit:
		if e.li >= e.cfg.outbound && e.cfg.outbound > 0 {
			return "RETURN"
		}
		return "TRANSIT"
	case StateScan:
		return "SCAN"
	case StateSense:
		return "SENSE"
	case StateDone:
		return "DONE"
	}
	return "UNKNOWN"
}

func (e *Engine) event(t time.Time, kind, detail string) {
	e.events = append(e.events, Event{T: t, Kind: kind, Detail: detail})
}

func (e *Engine) drainEvents() []Event {
	ev := e.events
	e.events = nil
	return ev
}

func buildLegs(p Params) []leg {
	tagOf := func(i int) int {
		if i < len(p.TagIDs) {
			return p.TagIDs[i]
		}
		return TagAny
	}
	n := len(p.Waypoints)
	legs := make([]leg, 0, 2*n)
	for i, wp := range p.Waypoints {
		legs = append(legs, leg{target: wp, tag: tagOf(i), wpIndex: i, sense: true})
	}
	if p.ReturnHome {
		for i := n - 2; i >= 0; i-- {
			legs = append(legs, leg{target: p.Waypoints[i], tag: tagOf(i), wpIndex: i})
		}
		legs = append(legs, leg{target: p.Start.Position(), tag: TagNone, wpIndex: -1})
	}
	return legs
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
