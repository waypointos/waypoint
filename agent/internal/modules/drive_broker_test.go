package modules

import (
	"context"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func driveCmdBytes(t *testing.T, vx float64) []byte {
	t.Helper()
	b, err := proto.Marshal(&waypointv1.DriveCommand{BodyVxMps: &vx})
	require.NoError(t, err)
	return b
}

// awaitMode polls until the broker's tracked mode matches, so tests don't race
// the async event.mode delivery.
func awaitMode(t *testing.T, b *driveBroker, want waypointv1.Mode) {
	t.Helper()
	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.mode == want
	}, 2*time.Second, 10*time.Millisecond, "broker never observed mode %v", want)
}

func TestDriveBroker_RelaysCmdInAutonomousMode(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "nav"))
	defer b.stop("nav")

	got := make(chan float64, 4)
	_, err := nc.Subscribe("waypoint.rov.cmd.drive", func(m *natsgo.Msg) {
		var c waypointv1.DriveCommand
		if proto.Unmarshal(m.Data, &c) == nil {
			got <- c.GetBodyVxMps()
		}
	})
	require.NoError(t, err)

	publishMode(t, nc, "rov", waypointv1.Mode_MODE_AUTONOMOUS)
	awaitMode(t, b, waypointv1.Mode_MODE_AUTONOMOUS)

	require.NoError(t, nc.Publish("waypoint.rov.module.nav.drive.cmd", driveCmdBytes(t, 0.4)))
	select {
	case vx := <-got:
		require.Equal(t, 0.4, vx)
	case <-time.After(2 * time.Second):
		t.Fatal("drive command was not relayed to cmd.drive in autonomous mode")
	}
}

func TestDriveBroker_GatesOutsideAutonomous(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "nav"))
	defer b.stop("nav")

	got := make(chan struct{}, 8)
	_, err := nc.Subscribe("waypoint.rov.cmd.drive", func(m *natsgo.Msg) { got <- struct{}{} })
	require.NoError(t, err)

	cmdB := driveCmdBytes(t, 0.4)
	mustNotRelay := func(why string) {
		for i := 0; i < 5; i++ {
			require.NoError(t, nc.Publish("waypoint.rov.module.nav.drive.cmd", cmdB))
		}
		select {
		case <-got:
			t.Fatal(why)
		case <-time.After(300 * time.Millisecond):
		}
	}

	// Default (unspecified mode) blocks before any mode event is seen.
	mustNotRelay("drive command must not be relayed before autonomous mode is set")

	for _, mode := range []waypointv1.Mode{
		waypointv1.Mode_MODE_MANUAL, waypointv1.Mode_MODE_SAFE, waypointv1.Mode_MODE_ESTOP,
	} {
		publishMode(t, nc, "rov", mode)
		awaitMode(t, b, mode)
		mustNotRelay("drive command must not be relayed in mode " + mode.String())
	}
}

func TestDriveBroker_ModeFlipStopsRelayMidStream(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "nav"))
	defer b.stop("nav")

	got := make(chan struct{}, 8)
	_, err := nc.Subscribe("waypoint.rov.cmd.drive", func(m *natsgo.Msg) { got <- struct{}{} })
	require.NoError(t, err)

	cmdB := driveCmdBytes(t, 0.4)
	publishMode(t, nc, "rov", waypointv1.Mode_MODE_AUTONOMOUS)
	awaitMode(t, b, waypointv1.Mode_MODE_AUTONOMOUS)
	require.NoError(t, nc.Publish("waypoint.rov.module.nav.drive.cmd", cmdB))
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("drive command was not relayed while autonomous")
	}

	// Estop mid-stream: subsequent commands must stop cold.
	publishMode(t, nc, "rov", waypointv1.Mode_MODE_ESTOP)
	awaitMode(t, b, waypointv1.Mode_MODE_ESTOP)
	for i := 0; i < 5; i++ {
		require.NoError(t, nc.Publish("waypoint.rov.module.nav.drive.cmd", cmdB))
	}
	select {
	case <-got:
		t.Fatal("drive command was relayed after the mode left autonomous")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDriveBroker_MirrorsTelemetryIntoModule(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "nav"))
	defer b.stop("nav")

	got := make(chan float64, 4)
	_, err := nc.Subscribe("waypoint.rov.module.nav.drive.telemetry", func(m *natsgo.Msg) {
		var tel waypointv1.DriveTelemetry
		if proto.Unmarshal(m.Data, &tel) == nil {
			got <- tel.GetBodyVxMps()
		}
	})
	require.NoError(t, err)

	vx := 0.25
	telB, _ := proto.Marshal(&waypointv1.DriveTelemetry{BodyVxMps: &vx})
	require.NoError(t, nc.Publish("waypoint.rov.telemetry.drive", telB))

	select {
	case v := <-got:
		require.Equal(t, 0.25, v)
	case <-time.After(2 * time.Second):
		t.Fatal("drive telemetry was not mirrored into the module sandbox")
	}
}

func TestDriveBroker_MirrorsModeEventsIntoModule(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.start(context.Background(), "nav"))
	defer b.stop("nav")

	got := make(chan waypointv1.Mode, 4)
	_, err := nc.Subscribe("waypoint.rov.module.nav.mode", func(m *natsgo.Msg) {
		var ev waypointv1.ModeEvent
		if proto.Unmarshal(m.Data, &ev) == nil {
			got <- ev.GetTo()
		}
	})
	require.NoError(t, err)

	publishMode(t, nc, "rov", waypointv1.Mode_MODE_AUTONOMOUS)

	select {
	case mode := <-got:
		require.Equal(t, waypointv1.Mode_MODE_AUTONOMOUS, mode)
	case <-time.After(2 * time.Second):
		t.Fatal("mode event was not mirrored into the module sandbox")
	}
}

// On dev images the per-module cmd relay is suppressed (the wildcard covers
// it) but an attached module still needs its mirrors: startMirrors must
// deliver mode and telemetry without opening a second cmd relay.
func TestDriveBroker_StartMirrorsDeliversWithoutCmdRelay(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.startMirrors(context.Background(), "nav"))
	defer b.stop("nav")

	gotMode := make(chan waypointv1.Mode, 4)
	_, err := nc.Subscribe("waypoint.rov.module.nav.mode", func(m *natsgo.Msg) {
		var ev waypointv1.ModeEvent
		if proto.Unmarshal(m.Data, &ev) == nil {
			gotMode <- ev.GetTo()
		}
	})
	require.NoError(t, err)
	relayed := make(chan struct{}, 1)
	_, err = nc.Subscribe("waypoint.rov.cmd.drive", func(*natsgo.Msg) {
		relayed <- struct{}{}
	})
	require.NoError(t, err)

	publishMode(t, nc, "rov", waypointv1.Mode_MODE_AUTONOMOUS)
	select {
	case mode := <-gotMode:
		require.Equal(t, waypointv1.Mode_MODE_AUTONOMOUS, mode)
	case <-time.After(2 * time.Second):
		t.Fatal("mode event was not mirrored by startMirrors")
	}

	vx := 0.3
	cmdB, _ := proto.Marshal(&waypointv1.DriveCommand{BodyVxMps: &vx})
	require.NoError(t, nc.Publish("waypoint.rov.module.nav.drive.cmd", cmdB))
	select {
	case <-relayed:
		t.Fatal("startMirrors must not relay drive.cmd")
	case <-time.After(300 * time.Millisecond):
	}
}

// startDev relays drive commands for any module id (dev images launch modules
// out-of-band, so no per-module attach fires) while keeping the mode gate.
func TestDriveBroker_DevWildcardRelaysAnyModuleAndKeepsModeGate(t *testing.T) {
	nc := startBus(t)
	b := newDriveBroker(nc, "rov")
	require.NoError(t, b.startDev(context.Background()))
	defer b.stop("*dev")

	got := make(chan struct{}, 8)
	_, err := nc.Subscribe("waypoint.rov.cmd.drive", func(m *natsgo.Msg) { got <- struct{}{} })
	require.NoError(t, err)

	cmdB := driveCmdBytes(t, 0.4)

	// Mode gate holds in dev too: nothing relays before autonomous.
	for i := 0; i < 5; i++ {
		require.NoError(t, nc.Publish("waypoint.rov.module.nav-sim.drive.cmd", cmdB))
	}
	select {
	case <-got:
		t.Fatal("dev wildcard relayed a drive command outside autonomous mode")
	case <-time.After(300 * time.Millisecond):
	}

	publishMode(t, nc, "rov", waypointv1.Mode_MODE_AUTONOMOUS)
	awaitMode(t, b, waypointv1.Mode_MODE_AUTONOMOUS)

	// A module id never attached still gets relayed via the wildcard.
	require.NoError(t, nc.Publish("waypoint.rov.module.nav-sim.drive.cmd", cmdB))
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("dev wildcard did not relay the drive command")
	}
}
