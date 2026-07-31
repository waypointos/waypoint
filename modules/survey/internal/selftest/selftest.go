// Package selftest exercises the full mission state machine against a
// scripted plant with a fake clock: actuators run 3% hot plus a slight yaw
// drift, so the trace demonstrates visual servoing and snap-on-sense
// correcting dead-reckoning error.
package selftest

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
	"github.com/waypointos/waypoint/modules/survey/internal/nav"
	"github.com/waypointos/waypoint/modules/survey/internal/vision"
)

const (
	step        = 10 * time.Millisecond
	odoEvery    = 20 * time.Millisecond  // 50 Hz telemetry
	tickEvery   = 50 * time.Millisecond  // 20 Hz control
	detEvery    = 100 * time.Millisecond // 10 Hz vision
	modeEvery   = time.Second
	maxSimTime  = 20 * time.Minute
	vxScaleErr  = 1.03  // plant moves 3% farther than odometry reports
	yawDriftRPS = 0.008 // uncommanded yaw the odometry does not see
)

// Run drives the scripted mission and writes the trace to w. It returns an
// error if the mission does not complete or a tag is misread.
func Run(w io.Writer) error {
	waypoints := []nav.Vec{{X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}}
	tags := []int{17, 42, 99}
	start := nav.Pose{}

	params := mission.Params{
		Waypoints:    waypoints,
		TagIDs:       tags,
		Start:        start,
		CruiseSpeed:  0.35,
		TurnRate:     0.8,
		ArriveRadius: 0.45,
		TagStop:      0.55,
		SenseDwell:   4 * time.Second,
		ScanMaxRad:   120 * math.Pi / 180,
		ReturnHome:   true,
	}
	eng := mission.New(params)

	truth := start
	sim := &vision.Sim{
		Waypoints:  waypoints,
		Tags:       tags,
		Visibility: 3.0,
		HfovDeg:    66,
		Pose:       func() nav.Pose { return truth },
	}

	now := time.Unix(1_700_000_000, 0)
	t0 := now
	var cmd mission.Drive
	var lastCmdAt time.Time
	detected := map[int]int{} // waypoint index -> id

	fmt.Fprintln(w, "selftest: 3 waypoints, tags 17/42/99, return home, vx scale err 1.03, yaw drift 0.008 rad/s")
	fmt.Fprintln(w, "selftest: mode MANUAL until t=1.0s, then AUTONOMOUS")

	elapsed := time.Duration(0)
	for ; elapsed < maxSimTime; elapsed += step {
		now = now.Add(step)

		// Mode mirror: manual for the first second, then autonomous, with
		// the platform's ~1 s re-announce.
		if elapsed%modeEvery == 0 {
			m := mission.ModeManual
			if elapsed >= time.Second {
				m = mission.ModeAutonomous
			}
			eng.OnMode(m, now)
		}

		// Plant: apply the last command with actuation error; the platform
		// failsafe zeroes motion 300 ms after the last publish.
		vx, yaw := cmd.VX, cmd.YawRate
		if now.Sub(lastCmdAt) > 300*time.Millisecond {
			vx, yaw = 0, 0
		}
		trueVX := vx * vxScaleErr
		trueYaw := yaw * vxScaleErr
		if vx != 0 || yaw != 0 {
			trueYaw += yawDriftRPS
		}
		truth.Integrate(trueVX, trueYaw, step.Seconds())

		if elapsed%odoEvery == 0 {
			// Odometry reports the commanded motion, blind to the errors.
			eng.OnOdometry(vx, yaw, now)
		}
		if elapsed%detEvery == 0 {
			if dets := sim.Detect(now); len(dets) > 0 {
				eng.OnDetections(dets, now)
			}
		}
		if elapsed%tickEvery != 0 {
			continue
		}

		out := eng.Tick(now)
		if out.Drive != nil && elapsed >= time.Second {
			cmd = *out.Drive
			lastCmdAt = now
		} else {
			cmd = mission.Drive{}
		}
		for _, ev := range out.Events {
			fmt.Fprintf(w, "%7.2fs  %-13s %s\n", ev.T.Sub(t0).Seconds(), ev.Kind, ev.Detail)
			if ev.Kind == "detected" {
				var wp, id int
				if _, err := fmt.Sscanf(ev.Detail, "wp=%d id=%d", &wp, &id); err == nil {
					detected[wp] = id
				}
			}
		}
		if out.State == "DONE" {
			break
		}
	}

	snap := eng.Snapshot()
	fmt.Fprintf(w, "selftest: finished at t=%.1fs state=%s\n", elapsed.Seconds(), snap.State)
	fmt.Fprintf(w, "selftest: believed pose (%.2f, %.2f) true pose (%.2f, %.2f), true dist to start %.2f m\n",
		snap.Pose.X, snap.Pose.Y, truth.X, truth.Y, nav.Dist(truth.Position(), start.Position()))

	if snap.State != "DONE" {
		return fmt.Errorf("selftest: mission did not complete within %s", maxSimTime)
	}
	for i, want := range tags {
		if got, ok := detected[i]; !ok {
			return fmt.Errorf("selftest: waypoint %d: no tag detected", i)
		} else if got != want {
			return fmt.Errorf("selftest: waypoint %d: detected %d, want %d", i, got, want)
		}
	}
	if d := nav.Dist(truth.Position(), start.Position()); d > 1.5 {
		return fmt.Errorf("selftest: ended %.2f m from start", d)
	}
	fmt.Fprintln(w, "selftest: PASS")
	return nil
}
