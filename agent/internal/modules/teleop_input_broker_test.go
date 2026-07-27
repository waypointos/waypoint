package modules

import (
	"context"
	"fmt"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func publishMode(t *testing.T, nc *natsgo.Conn, rover string, to waypointv1.Mode) {
	t.Helper()
	b, err := proto.Marshal(&waypointv1.ModeEvent{To: to})
	require.NoError(t, err)
	require.NoError(t, nc.Publish(fmt.Sprintf("waypoint.%s.event.mode", rover), b))
}

func TestTeleopInputBroker_MirrorsGamepadInManualMode(t *testing.T) {
	nc := startBus(t)
	b := newTeleopInputBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "so100"))
	defer b.stop("so100")

	got := make(chan []float32, 4)
	_, err := nc.Subscribe("waypoint.rov.module.so100.input", func(m *natsgo.Msg) {
		var g waypointv1.GamepadSnapshot
		if proto.Unmarshal(m.Data, &g) == nil {
			got <- g.GetAxes()
		}
	})
	require.NoError(t, err)

	publishMode(t, nc, "rov", waypointv1.Mode_MODE_MANUAL)

	// Mode propagates asynchronously; re-publish the snapshot until it mirrors.
	snapB, _ := proto.Marshal(&waypointv1.GamepadSnapshot{Axes: []float32{0.1, -0.2, 0.3, 0.4}})
	var gotAxes []float32
	require.Eventually(t, func() bool {
		_ = nc.Publish("waypoint.rov.cmd.gamepad", snapB)
		select {
		case gotAxes = <-got:
			return true
		default:
			return false
		}
	}, 2*time.Second, 20*time.Millisecond, "gamepad snapshot was not mirrored in manual mode")
	require.Equal(t, []float32{0.1, -0.2, 0.3, 0.4}, gotAxes)
}

func TestTeleopInputBroker_GatesOutsideManual(t *testing.T) {
	nc := startBus(t)
	b := newTeleopInputBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "so100"))
	defer b.stop("so100")

	got := make(chan struct{}, 4)
	_, err := nc.Subscribe("waypoint.rov.module.so100.input", func(m *natsgo.Msg) { got <- struct{}{} })
	require.NoError(t, err)

	snapB, _ := proto.Marshal(&waypointv1.GamepadSnapshot{Axes: []float32{0.1}})
	mustNotMirror := func(why string) {
		for i := 0; i < 5; i++ {
			require.NoError(t, nc.Publish("waypoint.rov.cmd.gamepad", snapB))
		}
		select {
		case <-got:
			t.Fatal(why)
		case <-time.After(300 * time.Millisecond):
		}
	}

	// Default (unspecified mode) blocks before any mode event is seen.
	mustNotMirror("gamepad must not be mirrored before manual mode is set")

	// Explicit SAFE also blocks.
	publishMode(t, nc, "rov", waypointv1.Mode_MODE_SAFE)
	time.Sleep(100 * time.Millisecond)
	mustNotMirror("gamepad must not be mirrored in safe mode")
}
