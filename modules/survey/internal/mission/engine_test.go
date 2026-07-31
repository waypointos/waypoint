package mission

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/waypointos/waypoint/modules/survey/internal/nav"
)

func testParams(waypoints []nav.Vec, tags []int, returnHome bool) Params {
	return Params{
		Waypoints:    waypoints,
		TagIDs:       tags,
		CruiseSpeed:  0.35,
		TurnRate:     0.8,
		ArriveRadius: 0.45,
		TagStop:      0.55,
		SenseDwell:   2 * time.Second,
		ScanMaxRad:   120 * math.Pi / 180,
		ReturnHome:   returnHome,
	}
}

// harness runs the engine against a perfect plant: commands are executed
// exactly and echoed back as odometry, so truth and belief coincide.
type harness struct {
	t      *testing.T
	e      *Engine
	now    time.Time
	pose   nav.Pose
	cmd    Drive
	mode   Mode
	events []Event
}

func newHarness(t *testing.T, p Params) *harness {
	return &harness{t: t, e: New(p), now: time.Unix(1000, 0), pose: p.Start, mode: ModeAutonomous}
}

// detFunc synthesizes detections from the plant's true pose.
type detFunc func(pose nav.Pose, now time.Time) []Detection

func (h *harness) step(dets detFunc) Output {
	h.now = h.now.Add(50 * time.Millisecond)
	h.e.OnMode(h.mode, h.now)
	h.pose.Integrate(h.cmd.VX, h.cmd.YawRate, 0.05)
	h.e.OnOdometry(h.cmd.VX, h.cmd.YawRate, h.now)
	if dets != nil {
		if d := dets(h.pose, h.now); len(d) > 0 {
			h.e.OnDetections(d, h.now)
		}
	}
	out := h.e.Tick(h.now)
	h.events = append(h.events, out.Events...)
	if out.Drive != nil {
		h.cmd = *out.Drive
	} else {
		h.cmd = Drive{}
	}
	return out
}

func (h *harness) runUntil(dets detFunc, pred func(Output) bool, maxSteps int) Output {
	var out Output
	for i := 0; i < maxSteps; i++ {
		out = h.step(dets)
		if pred(out) {
			return out
		}
	}
	h.t.Fatalf("condition not reached in %d steps (state %s)", maxSteps, out.State)
	return out
}

func (h *harness) eventKinds() map[string]int {
	m := map[string]int{}
	for _, ev := range h.events {
		m[ev.Kind]++
	}
	return m
}

// tagAt emits a geometric detection for a marker on a waypoint whenever it
// is within range and the camera FOV, mirroring the sim vision source.
func tagAt(wp nav.Vec, id int, visibility float64) detFunc {
	return func(pose nav.Pose, now time.Time) []Detection {
		d := nav.Dist(pose.Position(), wp)
		b := nav.BearingTo(pose, wp)
		if d > visibility || math.Abs(b) > 33*math.Pi/180 {
			return nil
		}
		return []Detection{{ID: id, Bearing: b, Distance: d, T: now}}
	}
}

func TestIdleUntilAutonomous(t *testing.T) {
	e := New(testParams([]nav.Vec{{X: 3, Y: 0}}, nil, false))
	now := time.Unix(1000, 0)

	out := e.Tick(now)
	assert.Nil(t, out.Drive)
	assert.Equal(t, "IDLE", out.State)
	assert.Equal(t, LEDGreen, out.LED.Phase)

	e.OnMode(ModeManual, now)
	out = e.Tick(now.Add(50 * time.Millisecond))
	assert.Equal(t, "IDLE", out.State)

	e.OnMode(ModeAutonomous, now.Add(100*time.Millisecond))
	out = e.Tick(now.Add(150 * time.Millisecond))
	assert.Equal(t, "TRANSIT", out.State)
	assert.Equal(t, LEDRed, out.LED.Phase)
	require.NotNil(t, out.Drive)
}

func TestNoWaypointsStaysIdle(t *testing.T) {
	e := New(testParams(nil, nil, true))
	now := time.Unix(1000, 0)
	e.OnMode(ModeAutonomous, now)
	out := e.Tick(now.Add(50 * time.Millisecond))
	assert.Equal(t, "IDLE", out.State)
	assert.Nil(t, out.Drive)
}

func TestTransitSteering(t *testing.T) {
	cases := []struct {
		name    string
		wp      nav.Vec
		wantVX  bool
		wantYaw float64 // sign; 0 = near zero
	}{
		{"ahead", nav.Vec{X: 5, Y: 0}, true, 0},
		{"left", nav.Vec{X: 0, Y: 5}, false, 1},
		{"right", nav.Vec{X: 0, Y: -5}, false, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, testParams([]nav.Vec{tc.wp}, nil, false))
			out := h.step(nil)
			require.NotNil(t, out.Drive)
			if tc.wantVX {
				assert.Positive(t, out.Drive.VX)
				assert.InDelta(t, 0, out.Drive.YawRate, 0.05)
			} else {
				// Large heading error: rotate in place first.
				assert.Zero(t, out.Drive.VX)
				assert.Equal(t, tc.wantYaw, math.Copysign(1, out.Drive.YawRate))
				assert.InDelta(t, 0.8, math.Abs(out.Drive.YawRate), 1e-9)
			}
		})
	}
}

func TestModePauseResume(t *testing.T) {
	h := newHarness(t, testParams([]nav.Vec{{X: 5, Y: 0}}, nil, false))
	out := h.step(nil)
	require.NotNil(t, out.Drive)

	h.mode = ModeManual
	out = h.step(nil)
	assert.Nil(t, out.Drive)
	assert.Equal(t, LEDGreen, out.LED.Phase)
	assert.Equal(t, 1, h.eventKinds()["pause"])

	h.mode = ModeAutonomous
	out = h.step(nil)
	require.NotNil(t, out.Drive)
	assert.Equal(t, LEDRed, out.LED.Phase)
	assert.Equal(t, 1, h.eventKinds()["resume"])
	assert.Equal(t, "TRANSIT", out.State)
}

func TestStaleModeMirrorPauses(t *testing.T) {
	e := New(testParams([]nav.Vec{{X: 5, Y: 0}}, nil, false))
	now := time.Unix(1000, 0)
	e.OnMode(ModeAutonomous, now)
	out := e.Tick(now.Add(50 * time.Millisecond))
	require.NotNil(t, out.Drive)

	// No re-announce for >2.5 s: treat as not-autonomous.
	out = e.Tick(now.Add(2600 * time.Millisecond))
	assert.Nil(t, out.Drive)
	assert.Equal(t, LEDGreen, out.LED.Phase)
}

func TestArrivalWithTagSensesAndSnaps(t *testing.T) {
	wp := nav.Vec{X: 3, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))
	dets := tagAt(wp, 17, 2.5)

	out := h.runUntil(dets, func(o Output) bool { return o.State == "SENSE" }, 2000)
	assert.Equal(t, LEDYellow, out.LED.Phase)
	// Arrived via the tag-stop distance, before the odometry radius.
	assert.LessOrEqual(t, nav.Dist(h.pose.Position(), wp), 0.60)

	out = h.runUntil(dets, func(o Output) bool { return o.State == "DONE" }, 200)
	kinds := h.eventKinds()
	assert.Equal(t, 1, kinds["detected"])
	assert.Zero(t, kinds["scan_start"])
	assert.Equal(t, 1, kinds["mission_end"])

	// Sense refixed the pose from the marker geometry: belief stays
	// consistent with the true standoff from the waypoint.
	snap := h.e.Snapshot()
	assert.InDelta(t, nav.Dist(h.pose.Position(), wp), nav.Dist(snap.Pose.Position(), wp), 0.05)
}

func TestSenseShowsBinaryID(t *testing.T) {
	wp := nav.Vec{X: 3, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))
	dets := tagAt(wp, 17, 2.5)
	h.runUntil(dets, func(o Output) bool { return o.State == "SENSE" }, 2000)
	out := h.step(dets) // one more tick so the dwell tallies a detection
	assert.True(t, out.LED.ShowID)
	assert.Equal(t, 17, out.LED.ID)
}

func TestSenseDwellDuration(t *testing.T) {
	wp := nav.Vec{X: 3, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))
	dets := tagAt(wp, 17, 2.5)
	h.runUntil(dets, func(o Output) bool { return o.State == "SENSE" }, 2000)
	start := h.now
	h.runUntil(dets, func(o Output) bool { return o.State == "DONE" }, 200)
	assert.GreaterOrEqual(t, h.now.Sub(start), 2*time.Second)
}

func TestWrongTagIgnored(t *testing.T) {
	wp := nav.Vec{X: 3, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))
	wrong := tagAt(wp, 99, 2.5)

	// The wrong id neither stops the approach early nor satisfies sensing:
	// arrival comes from odometry and the dwell ends with no detection.
	h.runUntil(wrong, func(o Output) bool { return o.State == "DONE" }, 3000)
	kinds := h.eventKinds()
	assert.Zero(t, kinds["detected"])
	assert.Equal(t, 1, kinds["no_detection"])
}

func TestArrivalWithoutTagScansThenGivesUp(t *testing.T) {
	wp := nav.Vec{X: 2, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))

	h.runUntil(nil, func(o Output) bool { return o.State == "SCAN" }, 2000)
	out := h.step(nil) // settled scan tick, past the arrival transition
	require.NotNil(t, out.Drive)
	assert.Zero(t, out.Drive.VX)
	assert.InDelta(t, 0.4, math.Abs(out.Drive.YawRate), 1e-9) // half turn rate

	// Sweep exhausts both directions, then the dwell runs without a tag
	// and the pose snaps to the surveyed waypoint.
	h.runUntil(nil, func(o Output) bool { return o.State == "DONE" }, 8000)
	kinds := h.eventKinds()
	assert.Equal(t, 1, kinds["scan_start"])
	assert.Equal(t, 1, kinds["scan_giveup"])
	assert.Equal(t, 1, kinds["no_detection"])
	snap := h.e.Snapshot()
	assert.Equal(t, wp.X, snap.Pose.X)
	assert.Equal(t, wp.Y, snap.Pose.Y)
}

func TestScanFindsTagMidSweep(t *testing.T) {
	wp := nav.Vec{X: 2, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))
	h.runUntil(nil, func(o Output) bool { return o.State == "SCAN" }, 2000)

	// Marker appears once the sweep points at it.
	dets := tagAt(wp, 17, 2.5)
	h.runUntil(dets, func(o Output) bool { return o.State == "SENSE" }, 400)
	assert.Equal(t, 1, h.eventKinds()["sense_start"])
}

func TestFullMissionWithReturn(t *testing.T) {
	wps := []nav.Vec{{X: 3, Y: 0}, {X: 3, Y: 3}}
	tags := []int{17, 42}
	h := newHarness(t, testParams(wps, tags, true))
	dets := func(pose nav.Pose, now time.Time) []Detection {
		var out []Detection
		for i, wp := range wps {
			out = append(out, tagAt(wp, tags[i], 2.5)(pose, now)...)
		}
		return out
	}

	sawReturn := false
	h.runUntil(dets, func(o Output) bool {
		if o.State == "RETURN" {
			sawReturn = true
		}
		return o.State == "DONE"
	}, 20000)

	assert.True(t, sawReturn, "return legs should report RETURN state")
	kinds := h.eventKinds()
	assert.Equal(t, 2, kinds["detected"], "one detection per outbound waypoint")
	assert.Equal(t, 1, kinds["mission_end"])
	// Legs: wp0, wp1, back to wp0, then start.
	assert.Equal(t, 4, kinds["arrived"])
	// Ends near the start point.
	assert.LessOrEqual(t, nav.Dist(h.pose.Position(), nav.Vec{}), 0.6)

	// DONE holds red and keeps publishing a zero setpoint while autonomous.
	out := h.step(dets)
	assert.Equal(t, LEDRed, out.LED.Phase)
	require.NotNil(t, out.Drive)
	assert.Zero(t, out.Drive.VX)

	// Operator takes over: engine rearms to IDLE, LED back to green.
	h.mode = ModeManual
	out = h.step(dets)
	assert.Equal(t, "IDLE", out.State)
	assert.Equal(t, LEDGreen, out.LED.Phase)
	assert.Equal(t, 1, h.eventKinds()["mission_reset"])
}

func TestSensePauseExtendsDwell(t *testing.T) {
	wp := nav.Vec{X: 3, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, []int{17}, false))
	dets := tagAt(wp, 17, 2.5)
	h.runUntil(dets, func(o Output) bool { return o.State == "SENSE" }, 2000)

	// Pause for a second mid-dwell; that time must not count.
	h.mode = ModeManual
	for i := 0; i < 20; i++ {
		h.step(dets)
	}
	h.mode = ModeAutonomous
	resumeAt := h.now
	h.runUntil(dets, func(o Output) bool { return o.State == "DONE" }, 200)
	assert.Greater(t, h.now.Sub(resumeAt), time.Second)
}

func TestSnapWithoutGeometry(t *testing.T) {
	wp := nav.Vec{X: 2, Y: 0}
	h := newHarness(t, testParams([]nav.Vec{wp}, nil, false))
	// Detection without distance (marker too small to size): id is
	// recorded but the pose falls back to the plain waypoint snap.
	dets := func(pose nav.Pose, now time.Time) []Detection {
		if nav.Dist(pose.Position(), wp) > 2.0 {
			return nil
		}
		return []Detection{{ID: 5, Bearing: nav.BearingTo(pose, wp), T: now}}
	}
	h.runUntil(dets, func(o Output) bool { return o.State == "DONE" }, 4000)
	assert.Equal(t, 1, h.eventKinds()["detected"])
	snap := h.e.Snapshot()
	assert.Equal(t, wp.X, snap.Pose.X)
	assert.Equal(t, wp.Y, snap.Pose.Y)
}
