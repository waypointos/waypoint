package conformance

// Module conformance: the REAL agent + core (sim) plus a module binary built
// from the SDK examples. Asserts the component-class contracts end to end:
// health, stats, state stream, commands acting through the servo broker, and
// estop freezing the component.

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sim/internal/harness"
)

// buildExample compiles an sdk example into t.TempDir and returns the path.
func buildExample(t *testing.T, rel string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), filepath.Base(rel))
	cmd := exec.Command("go", "build", "-o", out, "./"+rel)
	cmd.Dir = "../../sdk"
	b, err := cmd.CombinedOutput()
	require.NoError(t, err, "build %s: %s", rel, string(b))
	return out
}

// benchPlatform returns the bench platformUT (skipping if pinned elsewhere).
func benchPlatform(t *testing.T) platformUT {
	t.Helper()
	for _, p := range platforms(t) {
		if !p.hasDrive() {
			return p
		}
	}
	t.Skip("bench platform not in matrix (WAYPOINT_DESCRIPTOR pinned to a drive platform)")
	return platformUT{}
}

func launchWithModule(t *testing.T, p platformUT, exampleRel, moduleID string, env ...string) (*harness.Rover, *harness.ModuleProc) {
	t.Helper()
	bin := buildExample(t, exampleRel)
	r := launch(t, true, p)
	mod, err := r.LaunchModule(bin, moduleID, env...)
	require.NoError(t, err)
	t.Cleanup(mod.Stop)
	return r, mod
}

// waitForMsg waits in WALL time for one decodable message on a leaf; module
// processes tick wall-clock even under the controlled core clock.
func waitForMsg[T proto.Message](t *testing.T, r *harness.Rover, leaf string, out T, timeout time.Duration) T {
	t.Helper()
	_, err := r.Trace.WaitFor(leaf, func(m harness.Msg) bool {
		return proto.Unmarshal(m.Data, out) == nil
	}, timeout)
	require.NoError(t, err, "no message on %s", leaf)
	return out
}

func TestModuleHealthAndStats(t *testing.T) {
	r, _ := launchWithModule(t, benchPlatform(t), "examples/sensor-minimal", "sensor-minimal",
		"WAYPOINT_MODULE_STATE_RATE_HZ=10")

	// health.ready answers over the rover's bus.
	require.Eventually(t, func() bool {
		msg, err := r.NC.Request("waypoint."+r.Opts.RoverID+".module.sensor-minimal.health.ready", nil, 300*time.Millisecond)
		return err == nil && string(msg.Data) == "ok"
	}, 10*time.Second, 200*time.Millisecond)

	st := waitForMsg(t, r, "module.sensor-minimal.stats", &waypointv1.ModuleStats{}, 10*time.Second)
	assert.NotZero(t, st.GetStamp().GetMonoNs(), "stats must carry a capture stamp")
}

func TestSensorConformance(t *testing.T) {
	r, _ := launchWithModule(t, benchPlatform(t), "examples/sensor-minimal", "sensor-minimal",
		"WAYPOINT_MODULE_STATE_RATE_HZ=10")

	sr := waitForMsg(t, r, "module.sensor-minimal.sensor.state", &waypointv1.SensorReadings{}, 10*time.Second)
	require.Len(t, sr.GetReadings(), 2)
	byName := map[string]*waypointv1.SensorReading{}
	for _, rd := range sr.GetReadings() {
		byName[rd.GetName()] = rd
	}
	assert.True(t, byName["bus_voltage"].GetOk())
	require.NotNil(t, byName["bus_voltage"].Value, "ok reading carries a value")
	assert.False(t, byName["water_depth"].GetOk(), "N/A reading: ok=false")
	assert.Nil(t, byName["water_depth"].Value, "N/A reading: absent value, not sentinel")
	assert.NotZero(t, sr.GetStamp().GetMonoNs())

	// Rate plausibility: at 10 Hz expect >= 5 messages in 1 s of wall time.
	r.Trace.Clear()
	time.Sleep(1 * time.Second)
	assert.GreaterOrEqual(t, len(r.Trace.Messages("module.sensor-minimal.sensor.state")), 5)
}

func TestMultiComponentStreams(t *testing.T) {
	// The sensor-specific var must pin the SDK sensor loop to 10 Hz even though
	// the global var says 20 Hz, proving per-class resolution end to end. The
	// example's generic probe loop is a fixed 50 ms ticker that reads no env.
	r, _ := launchWithModule(t, benchPlatform(t), "examples/multi-component", "multi-component",
		"WAYPOINT_MODULE_STATE_RATE_HZ=20",
		"WAYPOINT_MODULE_STATE_RATE_HZ_SENSOR=10")

	sr := waitForMsg(t, r, "module.multi-component.sensor.state", &waypointv1.SensorReadings{}, 10*time.Second)
	require.NotZero(t, sr.GetStamp().GetMonoNs())

	_, err := r.Trace.WaitFor("module.multi-component.probe.state", func(m harness.Msg) bool { return true }, 10*time.Second)
	require.NoError(t, err, "generic probe.state stream must flow alongside sensor.state")

	// Rate over 1 s of wall time: sensor ~10 Hz, probe ~20 Hz. The sensor upper
	// bound is what fails if the per-class var is ignored and the loop runs at
	// the global 20 Hz.
	r.Trace.Clear()
	time.Sleep(1 * time.Second)
	sensorCount := len(r.Trace.Messages("module.multi-component.sensor.state"))
	assert.GreaterOrEqual(t, sensorCount, 5)
	assert.LessOrEqual(t, sensorCount, 15, "sensor.state must honor the per-class 10 Hz, not the global 20 Hz")
	probeCount := len(r.Trace.Messages("module.multi-component.probe.state"))
	assert.GreaterOrEqual(t, probeCount, 10)
	assert.LessOrEqual(t, probeCount, 30)
}

func TestArmConformanceCommandActs(t *testing.T) {
	r, _ := launchWithModule(t, benchPlatform(t), "examples/arm-sim", "arm-sim",
		"WAYPOINT_MODULE_STATE_RATE_HZ=10")

	// Arm state flows with the bench joint set.
	st := waitForMsg(t, r, "module.arm-sim.arm.state", &waypointv1.ArmState{}, 10*time.Second)
	require.Len(t, st.GetJoints(), 6)
	assert.Equal(t, "arm_1", st.GetJoints()[0].GetName())

	// Command: move arm_1 to +0.5 rad. The module sends servo ops through the
	// broker; core (controlled clock) needs Advance to run the physics.
	cmd := &waypointv1.ArmCommand{Cmd: &waypointv1.ArmCommand_Goals{Goals: &waypointv1.ArmJointGoals{
		Goals: []*waypointv1.ArmJointGoal{{Name: "arm_1", PositionRad: 0.5}},
	}}}
	b, err := proto.Marshal(cmd)
	require.NoError(t, err)
	require.NoError(t, r.NC.Publish("waypoint."+r.Opts.RoverID+".module.arm-sim.arm.cmd", b))
	require.NoError(t, r.NC.Flush())

	// Give the module wall time to relay, then advance core physics in chunks,
	// polling the module's reported state between grants.
	deadline := time.Now().Add(20 * time.Second)
	reached := false
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond) // module relay + state publish (wall)
		require.NoError(t, r.Advance(200*time.Millisecond))
		r.Trace.Clear()
		st := waitForMsg(t, r, "module.arm-sim.arm.state", &waypointv1.ArmState{}, 3*time.Second)
		for _, j := range st.GetJoints() {
			if j.GetName() == "arm_1" && j.GetPositionRad() > 0.4 {
				reached = true
			}
		}
		if reached {
			break
		}
	}
	assert.True(t, reached, "arm_1 must converge toward 0.5 rad via the broker")
}

// A standard stop during motion must freeze the arm where it is: the module
// re-latches every goal to its present position, so the move ends well short
// of the original target and the position holds afterward.
func TestArmConformanceStopHaltsMotion(t *testing.T) {
	r, _ := launchWithModule(t, benchPlatform(t), "examples/arm-sim", "arm-sim",
		"WAYPOINT_MODULE_STATE_RATE_HZ=10")
	waitForMsg(t, r, "module.arm-sim.arm.state", &waypointv1.ArmState{}, 10*time.Second)

	// Command a long move: 2.0 rad is raw goal ~3352 from center 2048.
	cmd := &waypointv1.ArmCommand{Cmd: &waypointv1.ArmCommand_Goals{Goals: &waypointv1.ArmJointGoals{
		Goals: []*waypointv1.ArmJointGoal{{Name: "arm_1", PositionRad: 2.0}},
	}}}
	b, err := proto.Marshal(cmd)
	require.NoError(t, err)
	require.NoError(t, r.NC.Publish("waypoint."+r.Opts.RoverID+".module.arm-sim.arm.cmd", b))
	require.NoError(t, r.NC.Flush())

	// Let the move get in flight: wall time for the relay, virtual for physics.
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, r.Advance(400*time.Millisecond))

	stop := &waypointv1.ArmCommand{Cmd: &waypointv1.ArmCommand_Stop{Stop: true}}
	sb, err := proto.Marshal(stop)
	require.NoError(t, err)
	require.NoError(t, r.NC.Publish("waypoint."+r.Opts.RoverID+".module.arm-sim.arm.cmd", sb))
	require.NoError(t, r.NC.Flush())
	time.Sleep(500 * time.Millisecond) // module reads present + re-latches (wall)
	require.NoError(t, r.Advance(200*time.Millisecond))

	frozen, err := r.ServoRead(1)
	require.NoError(t, err)
	require.True(t, frozen.GetOk())

	// Further time must not resume the original move.
	require.NoError(t, r.Advance(1*time.Second))
	after, err := r.ServoRead(1)
	require.NoError(t, err)
	require.True(t, after.GetOk())
	assert.InDelta(t, int(frozen.GetPositionRaw()), int(after.GetPositionRaw()), 12,
		"position must hold after stop")
	assert.Less(t, int(after.GetPositionRaw()), 3252,
		"stop must land well short of the original ~3352 goal")
}

func TestArmConformanceEstopFreezes(t *testing.T) {
	r, _ := launchWithModule(t, benchPlatform(t), "examples/arm-sim", "arm-sim",
		"WAYPOINT_MODULE_STATE_RATE_HZ=10")

	// Wait for the module to be live, then estop the platform.
	waitForMsg(t, r, "module.arm-sim.arm.state", &waypointv1.ArmState{}, 10*time.Second)
	require.NoError(t, r.Estop())
	require.NoError(t, r.Advance(300*time.Millisecond))
	_, err := r.Trace.WaitFor("event.mode", func(m harness.Msg) bool {
		var me waypointv1.ModeEvent
		return proto.Unmarshal(m.Data, &me) == nil && me.GetTo() == waypointv1.Mode_MODE_ESTOP
	}, 2*time.Second)
	require.NoError(t, err)

	// Baseline position under estop.
	before, err := r.ServoRead(1)
	require.NoError(t, err)
	require.True(t, before.GetOk())

	// Component command during estop: must not move the servo. Core gates the
	// goal write; this proves the guarantee holds through SDK + broker.
	cmd := &waypointv1.ArmCommand{Cmd: &waypointv1.ArmCommand_Goals{Goals: &waypointv1.ArmJointGoals{
		Goals: []*waypointv1.ArmJointGoal{{Name: "arm_1", PositionRad: 1.0}},
	}}}
	b, _ := proto.Marshal(cmd)
	require.NoError(t, r.NC.Publish("waypoint."+r.Opts.RoverID+".module.arm-sim.arm.cmd", b))
	require.NoError(t, r.NC.Flush())
	time.Sleep(300 * time.Millisecond) // wall time for module -> broker -> core
	require.NoError(t, r.Advance(500*time.Millisecond))

	after, err := r.ServoRead(1)
	require.NoError(t, err)
	require.True(t, after.GetOk())
	assert.InDelta(t, int(before.GetPositionRaw()), int(after.GetPositionRaw()), 8,
		"estop must freeze the arm: goal writes gated in core")
}
