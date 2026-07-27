package wpmodule

import (
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// fakeM builds an M directly against a test server, bypassing Run.
func fakeM(t *testing.T, url, roverID, id string) *M {
	t.Helper()
	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return &M{id: id, roverID: roverID, nc: nc, start: time.Now()}
}

func TestServoClientPublishesControlOps(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()
	got := make(chan *waypointv1.ServoControl, 4)
	_, err = obs.Subscribe("waypoint.dev.module.demo.servo.cmd", func(msg *natsgo.Msg) {
		var c waypointv1.ServoControl
		if proto.Unmarshal(msg.Data, &c) == nil {
			got <- &c
		}
	})
	require.NoError(t, err)
	require.NoError(t, obs.Flush())

	sv := m.Servo()
	require.NoError(t, sv.SetTorqueEnable(3, true))
	require.NoError(t, sv.SetGoalPosition(3, 2100))

	c1 := <-got
	assert.Equal(t, uint32(3), c1.GetServoId())
	assert.True(t, c1.GetSetTorqueEnable())
	c2 := <-got
	assert.Equal(t, uint32(2100), c2.GetSetGoalPosition())
}

func TestServoClientPublishesNegativeGoalVelocity(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()
	got := make(chan *waypointv1.ServoControl, 1)
	_, err = obs.Subscribe("waypoint.dev.module.demo.servo.cmd", func(msg *natsgo.Msg) {
		var c waypointv1.ServoControl
		if proto.Unmarshal(msg.Data, &c) == nil {
			got <- &c
		}
	})
	require.NoError(t, err)
	require.NoError(t, obs.Flush())

	require.NoError(t, m.Servo().SetGoalVelocity(11, -1500))

	c := <-got
	assert.Equal(t, uint32(11), c.GetServoId())
	op, ok := c.GetOp().(*waypointv1.ServoControl_SetGoalVelocity)
	require.True(t, ok)
	assert.Equal(t, int32(-1500), op.SetGoalVelocity)
	assert.Equal(t, int32(-1500), c.GetSetGoalVelocity())
}

func TestServoClientReadRoundTrip(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")

	// Fake broker responder on the module's read subject.
	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()
	_, err = obs.Subscribe("waypoint.dev.module.demo.servo.read", func(msg *natsgo.Msg) {
		st := &waypointv1.ServoState{ServoId: 3, Ok: true, PositionRaw: proto.Uint32(2048)}
		b, _ := proto.Marshal(st)
		_ = msg.Respond(b)
	})
	require.NoError(t, err)
	require.NoError(t, obs.Flush())

	st, err := m.Servo().Read(3)
	require.NoError(t, err)
	assert.True(t, st.GetOk())
	assert.Equal(t, uint32(2048), st.GetPositionRaw())
}
