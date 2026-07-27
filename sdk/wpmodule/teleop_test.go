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

func TestTeleopInputDeliversSnapshots(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")

	got := make(chan *waypointv1.GamepadSnapshot, 1)
	sub, err := m.TeleopInput(func(s *waypointv1.GamepadSnapshot) {
		select {
		case got <- s:
		default:
		}
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()
	b, err := proto.Marshal(&waypointv1.GamepadSnapshot{Buttons: []bool{true}})
	require.NoError(t, err)
	require.NoError(t, obs.Publish("waypoint.dev.module.demo.input", b))
	require.NoError(t, obs.Flush())

	select {
	case s := <-got:
		assert.Equal(t, []bool{true}, s.GetButtons())
	case <-time.After(2 * time.Second):
		t.Fatal("no GamepadSnapshot within 2s")
	}
}

func TestPublishUplink(t *testing.T) {
	_, url := startServer(t)
	m := fakeM(t, url, "dev", "demo")

	obs, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer obs.Close()
	got := make(chan *waypointv1.UplinkTelemetry, 1)
	_, err = obs.Subscribe("waypoint.dev.module.demo.uplink", func(msg *natsgo.Msg) {
		var u waypointv1.UplinkTelemetry
		if proto.Unmarshal(msg.Data, &u) == nil {
			select {
			case got <- &u:
			default:
			}
		}
	})
	require.NoError(t, err)
	require.NoError(t, obs.Flush())

	require.NoError(t, m.PublishUplink(&waypointv1.UplinkTelemetry{}))

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("no UplinkTelemetry within 2s")
	}
}
