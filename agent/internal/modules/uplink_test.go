package modules

import (
	"context"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestUplinkMirror_ForwardsValidPayload(t *testing.T) {
	srv, err := nats.StartEmbedded(nats.Options{Port: -1})
	require.NoError(t, err)
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.WaitReady(ctx))

	cli, err := natsgo.Connect(srv.ClientURL())
	require.NoError(t, err)
	defer cli.Close()

	var mu sync.Mutex
	var got []*waypointv1.UplinkTelemetry
	_, err = cli.Subscribe("waypoint.r1.telemetry.uplink", func(m *natsgo.Msg) {
		u := &waypointv1.UplinkTelemetry{}
		_ = proto.Unmarshal(m.Data, u)
		mu.Lock()
		got = append(got, u)
		mu.Unlock()
	})
	require.NoError(t, err)

	mir := newUplinkMirror(cli, "r1")
	require.NoError(t, mir.start(ctx, "umr"))

	bars := uint32(3)
	body, _ := proto.Marshal(&waypointv1.UplinkTelemetry{SignalBars: &bars})
	require.NoError(t, cli.Publish("waypoint.r1.module.umr.uplink", body))
	// garbage must be dropped, not forwarded
	require.NoError(t, cli.Publish("waypoint.r1.module.umr.uplink", []byte{0xff, 0xff, 0xff}))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 1 && got[0].GetSignalBars() == 3
	}, time.Second, 10*time.Millisecond)

	mir.stop("umr")
	mu.Lock()
	n := len(got)
	mu.Unlock()
	require.NoError(t, cli.Publish("waypoint.r1.module.umr.uplink", body))
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	require.Equal(t, n, len(got), "no forwarding after stop")
	mu.Unlock()
}
