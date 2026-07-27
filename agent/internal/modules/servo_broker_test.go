package modules

import (
	"context"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func startBus(t *testing.T) *natsgo.Conn {
	t.Helper()
	srv, err := nats.StartEmbedded(nats.Options{Port: -1})
	require.NoError(t, err)
	t.Cleanup(srv.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, srv.WaitReady(ctx))
	nc, err := natsgo.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

func TestServoBroker_RelaysArmServoAndGuardsDriveWheels(t *testing.T) {
	nc := startBus(t)
	b := newServoBroker(nc, "rov", defaultDenyIDs)
	require.NoError(t, b.start(context.Background(), "so100"))

	got := make(chan uint32, 4)
	_, err := nc.Subscribe("waypoint.rov.cmd.servo", func(m *natsgo.Msg) {
		var c waypointv1.ServoControl
		if proto.Unmarshal(m.Data, &c) == nil {
			got <- c.GetServoId()
		}
	})
	require.NoError(t, err)

	arm := &waypointv1.ServoControl{ServoId: 3, Op: &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: 2048}}
	armB, _ := proto.Marshal(arm)
	require.NoError(t, nc.Publish("waypoint.rov.module.so100.servo.cmd", armB))

	wheel := &waypointv1.ServoControl{ServoId: 7, Op: &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: 2048}}
	wheelB, _ := proto.Marshal(wheel)
	require.NoError(t, nc.Publish("waypoint.rov.module.so100.servo.cmd", wheelB))

	select {
	case id := <-got:
		require.Equal(t, uint32(3), id)
	case <-time.After(time.Second):
		t.Fatal("arm servo command was not relayed to cmd.servo")
	}
	select {
	case id := <-got:
		t.Fatalf("drive wheel %d was relayed but must be guarded", id)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing more arrives
	}
}

func TestServoBroker_RelaysSyncAndGuardsDriveWheels(t *testing.T) {
	nc := startBus(t)
	b := newServoBroker(nc, "rov", defaultDenyIDs)
	require.NoError(t, b.start(context.Background(), "so100"))

	got := make(chan []uint32, 2)
	_, err := nc.Subscribe("waypoint.rov.cmd.servo_sync", func(m *natsgo.Msg) {
		var s waypointv1.ServoSyncWrite
		if proto.Unmarshal(m.Data, &s) == nil {
			ids := []uint32{}
			for _, g := range s.GetGoals() {
				ids = append(ids, g.GetServoId())
			}
			got <- ids
		}
	})
	require.NoError(t, err)

	sw := &waypointv1.ServoSyncWrite{Goals: []*waypointv1.ServoGoal{
		{ServoId: 3, GoalPosition: 2048},
		{ServoId: 9, GoalPosition: 1000}, // drive wheel, must be dropped
	}}
	b2, _ := proto.Marshal(sw)
	require.NoError(t, nc.Publish("waypoint.rov.module.so100.servo.sync", b2))

	select {
	case ids := <-got:
		require.Equal(t, []uint32{3}, ids) // wheel 9 filtered out
	case <-time.After(2 * time.Second):
		t.Fatal("sync write was not relayed")
	}
}

func TestServoBroker_ReadProxiesToCore(t *testing.T) {
	nc := startBus(t)
	b := newServoBroker(nc, "rov", defaultDenyIDs)
	require.NoError(t, b.start(context.Background(), "so100"))

	// Fake core: answer rpc.servo_read with a ServoState echoing the id.
	_, err := nc.Subscribe("waypoint.rov.rpc.servo_read", func(m *natsgo.Msg) {
		var req waypointv1.ServoReadRequest
		require.NoError(t, proto.Unmarshal(m.Data, &req))
		st := &waypointv1.ServoState{ServoId: req.GetServoId(), Ok: true}
		body, _ := proto.Marshal(st)
		_ = m.Respond(body)
	})
	require.NoError(t, err)

	reqBody, _ := proto.Marshal(&waypointv1.ServoReadRequest{ServoId: 3})
	resp, err := nc.Request("waypoint.rov.module.so100.servo.read", reqBody, time.Second)
	require.NoError(t, err)

	var st waypointv1.ServoState
	require.NoError(t, proto.Unmarshal(resp.Data, &st))
	require.True(t, st.GetOk())
	require.Equal(t, uint32(3), st.GetServoId())
}

// startDev brokers servo subjects for any module id (dev images launch modules
// out-of-band, so no per-module attach fires) while keeping the drive guard.
func TestServoBroker_DevWildcardRelaysAnyModuleAndGuardsDriveWheels(t *testing.T) {
	nc := startBus(t)
	b := newServoBroker(nc, "rov", defaultDenyIDs)
	require.NoError(t, b.startDev(context.Background()))

	got := make(chan uint32, 4)
	_, err := nc.Subscribe("waypoint.rov.cmd.servo", func(m *natsgo.Msg) {
		var c waypointv1.ServoControl
		if proto.Unmarshal(m.Data, &c) == nil {
			got <- c.GetServoId()
		}
	})
	require.NoError(t, err)

	// A module id never attached still gets relayed via the wildcard.
	arm := &waypointv1.ServoControl{ServoId: 3, Op: &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: 2048}}
	armB, _ := proto.Marshal(arm)
	require.NoError(t, nc.Publish("waypoint.rov.module.arm-sim.servo.cmd", armB))

	wheel := &waypointv1.ServoControl{ServoId: 7, Op: &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: 2048}}
	wheelB, _ := proto.Marshal(wheel)
	require.NoError(t, nc.Publish("waypoint.rov.module.arm-sim.servo.cmd", wheelB))

	select {
	case id := <-got:
		require.Equal(t, uint32(3), id)
	case <-time.After(time.Second):
		t.Fatal("dev wildcard did not relay the arm servo command")
	}
	select {
	case id := <-got:
		t.Fatalf("drive wheel %d was relayed but must be guarded", id)
	case <-time.After(200 * time.Millisecond):
	}
}
