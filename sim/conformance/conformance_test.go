package conformance

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sim/internal/harness"
)

// lastMotorVels returns the most recent velocity_radps per drive-wheel id seen
// in the trace. telemetry.motors emits one message per servo per tick.
func lastMotorVels(tr *harness.Trace, wheelIDs map[uint32]bool) map[uint32]float64 {
	out := map[uint32]float64{}
	for _, m := range tr.Messages("telemetry.motors") {
		var mt waypointv1.MotorTelemetry
		if proto.Unmarshal(m.Data, &mt) != nil {
			continue
		}
		if wheelIDs[mt.GetId()] {
			out[mt.GetId()] = mt.GetVelocityRadps()
		}
	}
	return out
}

// lastAbsDriveVel is the largest |velocity| among each wheel's most-recent
// (steady-state) telemetry sample. Use after motion has settled.
func lastAbsDriveVel(tr *harness.Trace, wheelIDs map[uint32]bool) float64 {
	max := 0.0
	for _, v := range lastMotorVels(tr, wheelIDs) {
		if a := math.Abs(v); a > max {
			max = a
		}
	}
	return max
}

// peakMotorVels is the largest |velocity| per drive-wheel id across the whole
// recorded window. Drive commands go stale after 300ms with no refresh, so a
// wheel spins up and back down within one Advance; the peak is the honest
// steady-state signal.
func peakMotorVels(tr *harness.Trace, wheelIDs map[uint32]bool) map[uint32]float64 {
	out := map[uint32]float64{}
	for _, m := range tr.Messages("telemetry.motors") {
		var mt waypointv1.MotorTelemetry
		if proto.Unmarshal(m.Data, &mt) != nil || !wheelIDs[mt.GetId()] {
			continue
		}
		if a := math.Abs(mt.GetVelocityRadps()); a > out[mt.GetId()] {
			out[mt.GetId()] = a
		}
	}
	return out
}

// peakAbsDriveVel is the largest |velocity| seen across all drive wheels in the
// whole recorded window.
func peakAbsDriveVel(tr *harness.Trace, wheelIDs map[uint32]bool) float64 {
	max := 0.0
	for _, v := range peakMotorVels(tr, wheelIDs) {
		if v > max {
			max = v
		}
	}
	return max
}

// driveVelSequences returns the ordered |velocity| samples per drive-wheel id.
func driveVelSequences(tr *harness.Trace, wheelIDs map[uint32]bool) map[uint32][]float64 {
	out := map[uint32][]float64{}
	for _, m := range tr.Messages("telemetry.motors") {
		var mt waypointv1.MotorTelemetry
		if proto.Unmarshal(m.Data, &mt) != nil || !wheelIDs[mt.GetId()] {
			continue
		}
		out[mt.GetId()] = append(out[mt.GetId()], math.Abs(mt.GetVelocityRadps()))
	}
	return out
}

func maxAbs(vs []float64) float64 {
	max := 0.0
	for _, v := range vs {
		if a := math.Abs(v); a > max {
			max = a
		}
	}
	return max
}

// 1. E-stop stops all motion: torque-off and command rejection in Estop.
func TestEstopStopsAllMotion(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)
			require.NoError(t, r.SetModeManual())
			require.NoError(t, r.Advance(100*time.Millisecond)) // let the mode switch settle

			if p.hasDrive() {
				wheelIDs := p.driveWheelIDs(t)
				require.NoError(t, r.Drive(0.5, 0))
				// Stay inside the 300ms drive-staleness window so wheels spin.
				require.NoError(t, r.Advance(200*time.Millisecond))
				assert.Greater(t, peakAbsDriveVel(r.Trace, wheelIDs), 0.1,
					"wheels should be moving in manual")
			}

			require.NoError(t, r.Estop())
			require.NoError(t, r.Advance(300*time.Millisecond)) // process + publish the transition
			_, err := r.Trace.WaitFor("event.mode", func(m harness.Msg) bool {
				var me waypointv1.ModeEvent
				return proto.Unmarshal(m.Data, &me) == nil && me.GetTo() == waypointv1.Mode_MODE_ESTOP
			}, 1*time.Second)
			require.NoError(t, err, "estop must transition to ESTOP")

			r.Trace.Clear()
			// Commands issued while estopped must not produce motion, and any
			// motion already underway must stop.
			seekID := p.seekServoID(t)
			require.NoError(t, r.Drive(0.5, 0))
			require.NoError(t, r.ServoControl(&waypointv1.ServoControl{
				ServoId: seekID,
				Op:      &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: 3000},
			}))
			require.NoError(t, r.Advance(500*time.Millisecond))

			if p.hasDrive() {
				assert.Less(t, peakAbsDriveVel(r.Trace, p.driveWheelIDs(t)), 0.5,
					"no drive wheel may move after estop, even with a fresh drive command")
			}
			st, err := r.ServoRead(seekID)
			require.NoError(t, err)
			if st.GetOk() {
				assert.InDelta(t, 2048, int(st.GetPositionRaw()), 8,
					"estop must gate the servo goal write, so position is unchanged")
			}
		})
	}
}

// 1b. E-stop stops module-owned wheel servos. The servo broker refuses module
// writes while estopped, so core's descriptor-driven sweep is the only path
// left; a wheel-mode STS3215 acts on a latched GOAL_SPEED even after
// torque-off, so the sweep must write zero as well as drop torque.
func TestEstopStopsModuleWheels(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			ids := p.moduleWheelIDs()
			if len(ids) == 0 {
				t.Skip("platform declares no module-owned wheel joints")
			}
			r := launch(t, true, p)
			require.NoError(t, r.SetModeManual())
			require.NoError(t, r.Advance(100*time.Millisecond))

			// raw 2000 ticks/s is ~3.07 rad/s at 4096 steps per revolution.
			for id := range ids {
				require.NoError(t, r.ServoControl(&waypointv1.ServoControl{
					ServoId: id,
					Op:      &waypointv1.ServoControl_SetGoalVelocity{SetGoalVelocity: 2000},
				}))
			}
			require.NoError(t, r.Advance(500*time.Millisecond))
			spinning := lastMotorVels(r.Trace, ids)
			require.Len(t, spinning, len(ids), "every module wheel reports telemetry")
			for id, v := range spinning {
				require.Greaterf(t, math.Abs(v), 1.0, "module wheel %d must spin before estop", id)
			}

			require.NoError(t, r.Estop())
			require.NoError(t, r.Advance(300*time.Millisecond))
			_, err := r.Trace.WaitFor("event.mode", func(m harness.Msg) bool {
				var me waypointv1.ModeEvent
				return proto.Unmarshal(m.Data, &me) == nil && me.GetTo() == waypointv1.Mode_MODE_ESTOP
			}, 1*time.Second)
			require.NoError(t, err, "estop must transition to ESTOP")

			// Let the first-order velocity model settle (5*tau = 250ms), then
			// sample a fresh window for the steady state.
			require.NoError(t, r.Advance(500*time.Millisecond))
			r.Trace.Clear()
			require.NoError(t, r.Advance(500*time.Millisecond))
			stopped := lastMotorVels(r.Trace, ids)
			require.Len(t, stopped, len(ids))
			for id, v := range stopped {
				assert.Lessf(t, math.Abs(v), 0.2, "module wheel %d must stop after estop", id)
			}
		})
	}
}

// 1c. Autonomous accepts drive commands: cmd.drive actuates the wheels exactly
// as it does in Manual.
func TestAutonomousDriveActuatesWheels(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			if !p.hasDrive() {
				t.Skip("platform has no drive capability")
			}
			wheelIDs := p.driveWheelIDs(t)
			r := launch(t, true, p)
			require.NoError(t, r.SetModeAutonomous())
			require.NoError(t, r.Advance(100*time.Millisecond))

			_, err := r.Trace.WaitFor("event.mode", func(m harness.Msg) bool {
				var me waypointv1.ModeEvent
				return proto.Unmarshal(m.Data, &me) == nil && me.GetTo() == waypointv1.Mode_MODE_AUTONOMOUS
			}, 1*time.Second)
			require.NoError(t, err, "set_mode must transition to AUTONOMOUS")

			require.NoError(t, r.Drive(0.5, 0))
			// Stay inside the 300ms drive-staleness window so wheels spin.
			require.NoError(t, r.Advance(200*time.Millisecond))
			assert.Greater(t, peakAbsDriveVel(r.Trace, wheelIDs), 0.1,
				"wheels should be moving in autonomous")
		})
	}
}

// 1d. E-stop from Autonomous halts motion: same guarantee as from Manual.
func TestEstopFromAutonomousHaltsMotion(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)
			require.NoError(t, r.SetModeAutonomous())
			require.NoError(t, r.Advance(100*time.Millisecond))

			if p.hasDrive() {
				wheelIDs := p.driveWheelIDs(t)
				require.NoError(t, r.Drive(0.5, 0))
				require.NoError(t, r.Advance(200*time.Millisecond))
				assert.Greater(t, peakAbsDriveVel(r.Trace, wheelIDs), 0.1,
					"wheels should be moving in autonomous")
			}

			require.NoError(t, r.Estop())
			require.NoError(t, r.Advance(300*time.Millisecond))
			_, err := r.Trace.WaitFor("event.mode", func(m harness.Msg) bool {
				var me waypointv1.ModeEvent
				return proto.Unmarshal(m.Data, &me) == nil && me.GetTo() == waypointv1.Mode_MODE_ESTOP
			}, 1*time.Second)
			require.NoError(t, err, "estop must transition to ESTOP")

			if p.hasDrive() {
				// Let the first-order velocity model settle, then sample a fresh
				// window: motion already underway must stop and fresh commands
				// must be rejected.
				require.NoError(t, r.Advance(500*time.Millisecond))
				r.Trace.Clear()
				require.NoError(t, r.Drive(0.5, 0))
				require.NoError(t, r.Advance(500*time.Millisecond))
				assert.Less(t, peakAbsDriveVel(r.Trace, p.driveWheelIDs(t)), 0.5,
					"no drive wheel may move after estop from autonomous")
			}
		})
	}
}

// 2. Heartbeat loss drops to safe: fault + alert + mode SAFE.
func TestHeartbeatLossDropsToSafe(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)
			require.NoError(t, r.SetModeManual())
			// Let the mode transition settle.
			require.NoError(t, r.Advance(100*time.Millisecond))

			require.NoError(t, r.AdvanceWithoutHeartbeats(700*time.Millisecond))

			_, err := r.Trace.WaitFor("event.fault", func(m harness.Msg) bool {
				var fe waypointv1.FaultEvent
				return proto.Unmarshal(m.Data, &fe) == nil && fe.GetCode() == "heartbeat_lost"
			}, 2*time.Second)
			require.NoError(t, err, "expected event.fault heartbeat_lost")

			_, err = r.Trace.WaitFor("alert.raised", func(m harness.Msg) bool {
				var ar waypointv1.AlertRaised
				return proto.Unmarshal(m.Data, &ar) == nil && ar.GetCode() == "heartbeat_lost"
			}, 2*time.Second)
			require.NoError(t, err, "expected alert.raised heartbeat_lost")

			_, err = r.Trace.WaitFor("event.mode", func(m harness.Msg) bool {
				var me waypointv1.ModeEvent
				return proto.Unmarshal(m.Data, &me) == nil && me.GetTo() == waypointv1.Mode_MODE_SAFE
			}, 2*time.Second)
			require.NoError(t, err, "expected event.mode to SAFE")
		})
	}
}

// 3. Drive staleness zeroes the target after >300ms with no fresh cmd.drive.
func TestDriveStalenessZeroesTarget(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			if !p.hasDrive() {
				t.Skip("platform has no drive capability")
			}
			wheelIDs := p.driveWheelIDs(t)
			r := launch(t, true, p)
			require.NoError(t, r.SetModeManual())
			require.NoError(t, r.Advance(100*time.Millisecond))
			require.NoError(t, r.Drive(0.5, 0))
			require.NoError(t, r.Advance(200*time.Millisecond)) // fresh: wheels move
			assert.Greater(t, peakAbsDriveVel(r.Trace, wheelIDs), 0.1, "wheels moving while drive fresh")

			r.Trace.Clear()
			// Advance with heartbeats but NO further drive commands: drive goes stale.
			require.NoError(t, r.Advance(600*time.Millisecond))

			_, err := r.Trace.WaitFor("event.fault", func(m harness.Msg) bool {
				var fe waypointv1.FaultEvent
				return proto.Unmarshal(m.Data, &fe) == nil && fe.GetCode() == "drive_command_stale"
			}, 2*time.Second)
			require.NoError(t, err, "expected event.fault drive_command_stale")

			// Let the zeroed target propagate through the first-order velocity model.
			r.Trace.Clear()
			require.NoError(t, r.Advance(600*time.Millisecond))
			assert.Less(t, lastAbsDriveVel(r.Trace, wheelIDs), 0.5, "velocities decay to ~0 after stale-zero")
		})
	}
}

// 3b. Drive staleness applies in Autonomous too: an autonomy stack that stops
// publishing cmd.drive must not leave the rover coasting on its last target.
func TestDriveStalenessZeroesTargetInAutonomous(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			if !p.hasDrive() {
				t.Skip("platform has no drive capability")
			}
			wheelIDs := p.driveWheelIDs(t)
			r := launch(t, true, p)
			require.NoError(t, r.SetModeAutonomous())
			require.NoError(t, r.Advance(100*time.Millisecond))
			require.NoError(t, r.Drive(0.5, 0))
			require.NoError(t, r.Advance(200*time.Millisecond)) // fresh: wheels move
			assert.Greater(t, peakAbsDriveVel(r.Trace, wheelIDs), 0.1, "wheels moving while drive fresh")

			r.Trace.Clear()
			// Advance with heartbeats but NO further drive commands: drive goes stale.
			require.NoError(t, r.Advance(600*time.Millisecond))

			_, err := r.Trace.WaitFor("event.fault", func(m harness.Msg) bool {
				var fe waypointv1.FaultEvent
				return proto.Unmarshal(m.Data, &fe) == nil && fe.GetCode() == "drive_command_stale"
			}, 2*time.Second)
			require.NoError(t, err, "expected event.fault drive_command_stale")

			// Let the zeroed target propagate through the first-order velocity model.
			r.Trace.Clear()
			require.NoError(t, r.Advance(600*time.Millisecond))
			assert.Less(t, lastAbsDriveVel(r.Trace, wheelIDs), 0.5, "velocities decay to ~0 after stale-zero")
		})
	}
}

// 4. Drive kinematics match the golden value. vx = 1.0 m/s, golden per-wheel
// |omega| derived from the descriptor via WheelSpeeds. telemetry.motors carries
// the RAW servo speed (the joint driver un-inverts only on its own read path,
// not on the bus read main publishes), so left wheels (invert) report the
// negated sign; assert on magnitude per wheel.
func TestDriveKinematicsMatchGolden(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			if !p.hasDrive() {
				t.Skip("platform has no drive capability")
			}
			wheelIDs := p.driveWheelIDs(t)
			golden := p.goldenWheelRadps(t)

			// Each invocation launches a fresh rover, drives forward, and returns
			// the per-wheel |velocity| sequence. The rover is stopped before
			// returning so a second back-to-back launch finds the shared NATS port
			// free.
			seq := func() map[uint32][]float64 {
				r := launch(t, true, p)
				defer r.Stop()
				require.NoError(t, r.SetModeManual())
				require.NoError(t, r.Advance(100*time.Millisecond))
				require.NoError(t, r.Drive(1.0, 0))
				// Climb toward steady state (5*tau = 250ms) inside the 300ms
				// staleness window. The grant is split into 200ms chunks.
				require.NoError(t, r.Advance(280*time.Millisecond))
				out := driveVelSequences(r.Trace, wheelIDs)
				require.Len(t, out, len(wheelIDs), "all drive wheels report telemetry")
				return out
			}

			a := seq()
			for id, vs := range a {
				peak := maxAbs(vs)
				assert.InEpsilonf(t, golden[id], peak, 0.05,
					"wheel %d converged |velocity_radps|=%.4f within 5%% of %.3f", id, peak, golden[id])
			}

			// Determinism: an identical second launch + identical advances yields
			// the identical magnitude sequence. The async cmd.drive can be consumed
			// one tick earlier or later relative to a virtual-time grant, so the
			// captured sample COUNT may differ by one at the window boundary; the
			// physics does not. Compare the common prefix element-wise.
			b := seq()
			for id, va := range a {
				vb := b[id]
				n := min(len(va), len(vb))
				require.GreaterOrEqualf(t, n, 4, "wheel %d: too few samples to compare", id)
				for i := 0; i < n; i++ {
					assert.InDeltaf(t, va[i], vb[i], 1e-9,
						"wheel %d sample %d must be deterministic across launches", id, i)
				}
			}
		})
	}
}

// 5. Hard-stop seek like so100: detect the upper and lower hard stops by a
// current spike or a position plateau, as the so100 calibration routine does.
func TestHardStopSeekLikeSo100(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)
			seekID := p.seekServoID(t)

			const (
				lowStop  = 1500
				highStop = 2600
				stallA   = 2.0
			)
			require.NoError(t, r.Scenario(&waypointv1.SimScenario{
				ServoId:          proto.Uint32(seekID),
				HardStopMinTicks: proto.Uint32(lowStop),
				HardStopMaxTicks: proto.Uint32(highStop),
				StallCurrentA:    proto.Float64(stallA),
			}))
			// Torque on first: position mode latches goal = present, then we step it.
			require.NoError(t, r.ServoControl(&waypointv1.ServoControl{
				ServoId: seekID,
				Op:      &waypointv1.ServoControl_SetTorqueEnable{SetTorqueEnable: true},
			}))
			require.NoError(t, r.Advance(100*time.Millisecond))

			up := seekStop(t, r, seekID, +20, highStop)
			assert.InDeltaf(t, highStop, up, 10, "upper stop detected near %d, got %d", highStop, up)

			down := seekStop(t, r, seekID, -20, lowStop)
			assert.InDeltaf(t, lowStop, down, 10, "lower stop detected near %d, got %d", lowStop, down)
		})
	}
}

// seekStop walks the goal position in `step` increments, detecting the hard stop
// by a stall-current spike (>= 1.5 A) or three consecutive equal positions.
// Returns the position at which the stop was detected.
func seekStop(t *testing.T, r *harness.Rover, id uint32, step int, _ int) int {
	t.Helper()
	// Re-anchor the goal at the present position before walking, so a prior
	// seek direction does not leave a goal already past a stop.
	st, err := r.ServoRead(id)
	require.NoError(t, err)
	require.True(t, st.GetOk())
	goal := int(st.GetPositionRaw())

	var lastPos, sameCount int
	lastPos = -1
	for i := 0; i < 300; i++ {
		goal += step
		if goal < 0 {
			goal = 0
		}
		if goal > 4095 {
			goal = 4095
		}
		require.NoError(t, r.ServoControl(&waypointv1.ServoControl{
			ServoId: id,
			Op:      &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: uint32(goal)},
		}))
		require.NoError(t, r.Advance(100*time.Millisecond))

		st, err := r.ServoRead(id)
		require.NoError(t, err)
		require.True(t, st.GetOk())
		pos := int(st.GetPositionRaw())
		currentA := float64(st.GetCurrentRaw()) * 0.0065

		if currentA >= 1.5 {
			return pos
		}
		if pos == lastPos {
			sameCount++
			if sameCount >= 3 {
				return pos
			}
		} else {
			sameCount = 0
		}
		lastPos = pos
	}
	t.Fatalf("hard stop not detected after seeking with step %d", step)
	return 0
}

// 6. Absent servo is N/A. Discovery happens at core boot, so absence is applied
// AFTER boot via scenario; assert no telemetry.motors for the id in the last
// window and that rpc.servo_read returns ok = false.
func TestAbsentServoIsNA(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)

			absentID := p.absentServoID()
			require.NoError(t, r.Scenario(&waypointv1.SimScenario{
				ServoId: proto.Uint32(absentID),
				Absent:  proto.Bool(true),
			}))
			require.NoError(t, r.Advance(2*time.Second))

			r.Trace.Clear()
			require.NoError(t, r.Advance(1*time.Second))
			for _, m := range r.Trace.Messages("telemetry.motors") {
				var mt waypointv1.MotorTelemetry
				if proto.Unmarshal(m.Data, &mt) == nil {
					assert.NotEqualf(t, absentID, mt.GetId(),
						"absent servo %d must not appear in telemetry.motors", absentID)
				}
			}

			st, err := r.ServoRead(absentID)
			require.NoError(t, err)
			assert.False(t, st.GetOk(), "rpc.servo_read for an absent servo must report ok=false")
		})
	}
}

// 7. Drive commands on a drive-less platform are inert: no servo motion, no
// telemetry.drive. Capability absence is silence, not an error.
func TestFixedBaseIgnoresDrive(t *testing.T) {
	for _, p := range platforms(t) {
		if p.hasDrive() {
			continue
		}
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)
			require.NoError(t, r.SetModeManual())
			require.NoError(t, r.Advance(100*time.Millisecond))

			r.Trace.Clear()
			require.NoError(t, r.Drive(0.8, 0))
			require.NoError(t, r.Advance(500*time.Millisecond))

			assert.Empty(t, r.Trace.Messages("telemetry.drive"),
				"fixed_base must not publish telemetry.drive")
			for _, m := range r.Trace.Messages("telemetry.motors") {
				var mt waypointv1.MotorTelemetry
				if proto.Unmarshal(m.Data, &mt) == nil {
					assert.InDeltaf(t, 0.0, mt.GetVelocityRadps(), 0.05,
						"servo %d must not move on cmd.drive", mt.GetId())
				}
			}
		})
	}
}

// 8. TestRecorderEpisode: on every platform, an episode started over the bus
// captures the descriptor-declared streams and the action commands published
// during it, and lands as a readable MCAP with a success-marked sidecar.
func TestRecorderEpisode(t *testing.T) {
	for _, p := range platforms(t) {
		p := p
		t.Run(p.id, func(t *testing.T) {
			r := launch(t, true, p)

			start, err := r.RecorderStart("conformance episode")
			require.NoError(t, err)
			require.True(t, start.Ok, start.Reason)

			// Produce action traffic appropriate to the platform's altitudes.
			if p.hasDrive() {
				require.NoError(t, r.Drive(0.2, 0))
			} else {
				require.NoError(t, r.ServoControl(&waypointv1.ServoControl{
					ServoId: p.seekServoID(t),
					Op:      &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: 2048},
				}))
			}
			require.NoError(t, r.Advance(500*time.Millisecond))

			stop, err := r.RecorderStop(true, "")
			require.NoError(t, err)
			require.True(t, stop.Ok, stop.Reason)
			require.NotNil(t, stop.Summary.Success)

			// The cmd channel must have captured the command we published.
			cmdSubject := "cmd.drive"
			if !p.hasDrive() {
				cmdSubject = "cmd.servo"
			}
			var cmdCount uint64
			topics := map[string]bool{}
			for _, s := range stop.Summary.Streams {
				topics[s.Subject] = true
				if s.Subject == cmdSubject {
					cmdCount = s.Count
				}
			}
			require.Greater(t, cmdCount, uint64(0), "no %s captured", cmdSubject)
			for _, st := range p.desc.Observations.Streams {
				require.True(t, topics[st.Subject], "missing declared stream %s", st.Subject)
			}

			// The artifact itself is a valid MCAP.
			f, err := os.Open(filepath.Join(r.EpisodesDir, start.EpisodeId+".mcap"))
			require.NoError(t, err)
			defer f.Close()
			mr, err := mcap.NewReader(f)
			require.NoError(t, err)
			info, err := mr.Info()
			require.NoError(t, err)
			require.NotEmpty(t, info.Channels)
		})
	}
}
