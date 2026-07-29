package wpmodule

import (
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// fakeM is defined in servo_test.go.

type fakeArm struct {
	mu    sync.Mutex
	goals []*waypointv1.ArmJointGoal
	stops int
}

func (f *fakeArm) State() *waypointv1.ArmState {
	return &waypointv1.ArmState{Joints: []*waypointv1.ArmJoint{
		{Name: "arm_1", PositionRad: 0.5, Calibrated: true},
	}}
}

func (f *fakeArm) Command(cmd *waypointv1.ArmCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g := cmd.GetGoals(); g != nil {
		f.goals = append(f.goals, g.GetGoals()...)
	}
	if cmd.GetStop() {
		f.stops++
	}
	return nil
}

func TestServeArmPublishesStateAndDispatchesCommands(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ", "50")

	arm := &fakeArm{}
	stopSrv, err := m.ServeArm(arm)
	require.NoError(t, err)
	defer stopSrv()

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()

	// State stream flows, stamped, with the declared joint.
	stCh := make(chan *waypointv1.ArmState, 1)
	_, err = obs.Subscribe("waypoint.dev.module.demo.arm.state", func(msg *natsgo.Msg) {
		var st waypointv1.ArmState
		if proto.Unmarshal(msg.Data, &st) == nil {
			select {
			case stCh <- &st:
			default:
			}
		}
	})
	require.NoError(t, err)
	select {
	case st := <-stCh:
		require.Len(t, st.GetJoints(), 1)
		assert.Equal(t, "arm_1", st.GetJoints()[0].GetName())
		assert.NotZero(t, st.GetStamp().GetMonoNs())
	case <-time.After(2 * time.Second):
		t.Fatal("no ArmState within 2s at 50 Hz")
	}

	// Commands dispatch.
	cmd := &waypointv1.ArmCommand{Cmd: &waypointv1.ArmCommand_Goals{Goals: &waypointv1.ArmJointGoals{
		Goals: []*waypointv1.ArmJointGoal{{Name: "arm_1", PositionRad: 1.0}},
	}}}
	b, _ := proto.Marshal(cmd)
	require.NoError(t, obs.Publish("waypoint.dev.module.demo.arm.cmd", b))
	require.NoError(t, obs.Flush())
	require.Eventually(t, func() bool {
		arm.mu.Lock()
		defer arm.mu.Unlock()
		return len(arm.goals) == 1
	}, 2*time.Second, 20*time.Millisecond)
}

type fakeSensor struct{}

func (fakeSensor) State() *waypointv1.SensorReadings {
	return &waypointv1.SensorReadings{Readings: []*waypointv1.SensorReading{
		{Name: "volts", Unit: "V", Ok: false}, // N/A form: no value
	}}
}

func TestServeSensorPublishesReadings(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ", "50")

	stopSrv, err := m.ServeSensor(fakeSensor{})
	require.NoError(t, err)
	defer stopSrv()

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()
	ch := make(chan *waypointv1.SensorReadings, 1)
	_, err = obs.Subscribe("waypoint.dev.module.demo.sensor.state", func(msg *natsgo.Msg) {
		var sr waypointv1.SensorReadings
		if proto.Unmarshal(msg.Data, &sr) == nil {
			select {
			case ch <- &sr:
			default:
			}
		}
	})
	require.NoError(t, err)
	select {
	case sr := <-ch:
		require.Len(t, sr.GetReadings(), 1)
		assert.False(t, sr.GetReadings()[0].GetOk())
		assert.Nil(t, sr.GetReadings()[0].Value)
	case <-time.After(2 * time.Second):
		t.Fatal("no SensorReadings within 2s")
	}
}

func TestServeSensorPrefersClassSpecificRate(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")
	// Global says 1 Hz; the sensor-specific var must win at 50 Hz.
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ", "1")
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ_SENSOR", "50")

	stopSrv, err := m.ServeSensor(fakeSensor{})
	require.NoError(t, err)
	defer stopSrv()

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()

	n := 0
	done := make(chan struct{}, 1)
	_, err = obs.Subscribe("waypoint.dev.module.demo.sensor.state", func(msg *natsgo.Msg) {
		n++
		if n >= 5 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})
	require.NoError(t, err)
	// 5 messages needs 5 s at the global 1 Hz but 100 ms at 50 Hz.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("class-specific 50 Hz rate not applied")
	}
}

func TestStateRateForFallbacks(t *testing.T) {
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ", "20")
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ_ARM", "")
	assert.Equal(t, time.Second/20, stateRateFor("arm"))
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ_ARM", "250") // out of range: fall through
	assert.Equal(t, time.Second/20, stateRateFor("arm"))
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ_ARM", "40")
	assert.Equal(t, time.Second/40, stateRateFor("arm"))
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ", "")
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ_ARM", "")
	assert.Equal(t, time.Second/10, stateRateFor("arm"))
}

type fakeBase struct {
	mu   sync.Mutex
	cmds []*waypointv1.BaseCommand
}

func (f *fakeBase) State() *waypointv1.BaseState {
	return &waypointv1.BaseState{BodyVxMps: 0.25, YawRateRadps: 0.1}
}

func (f *fakeBase) Command(cmd *waypointv1.BaseCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, cmd)
	return nil
}

func TestServeBasePublishesStateAndDispatchesCommands(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")
	t.Setenv("WAYPOINT_MODULE_STATE_RATE_HZ", "50")

	base := &fakeBase{}
	stopSrv, err := m.ServeBase(base)
	require.NoError(t, err)
	defer stopSrv()

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()

	stCh := make(chan *waypointv1.BaseState, 1)
	_, err = obs.Subscribe("waypoint.dev.module.demo.base.state", func(msg *natsgo.Msg) {
		var st waypointv1.BaseState
		if proto.Unmarshal(msg.Data, &st) == nil {
			select {
			case stCh <- &st:
			default:
			}
		}
	})
	require.NoError(t, err)
	select {
	case st := <-stCh:
		assert.Equal(t, 0.25, st.GetBodyVxMps())
		assert.NotZero(t, st.GetStamp().GetMonoNs())
	case <-time.After(2 * time.Second):
		t.Fatal("no BaseState within 2s")
	}

	cmd := &waypointv1.BaseCommand{BodyVxMps: 0.5, YawRateRadps: -0.2}
	b, _ := proto.Marshal(cmd)
	require.NoError(t, obs.Publish("waypoint.dev.module.demo.base.cmd", b))
	require.NoError(t, obs.Flush())
	require.Eventually(t, func() bool {
		base.mu.Lock()
		defer base.mu.Unlock()
		return len(base.cmds) == 1
	}, 2*time.Second, 20*time.Millisecond)
}
