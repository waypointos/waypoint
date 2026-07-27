package wpmodule

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// startServer runs an in-process NATS server on an ephemeral port.
func startServer(t *testing.T) (*natsserver.Server, string) {
	t.Helper()
	opts := &natsserver.Options{Host: "127.0.0.1", Port: -1}
	s, err := natsserver.NewServer(opts)
	require.NoError(t, err)
	go s.Start()
	require.True(t, s.ReadyForConnections(5*time.Second))
	t.Cleanup(s.Shutdown)
	return s, s.ClientURL()
}

func TestRunServesHealthAndStats(t *testing.T) {
	_, url := startServer(t)
	t.Setenv("WAYPOINT_NATS_URL", url)
	t.Setenv("WAYPOINT_ROVER_ID", "dev")
	t.Setenv("WAYPOINT_MODULE_CREDS", "") // dev mode: plain connect

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{ID: "demo", StatsInterval: 50 * time.Millisecond}, func(m *M) error { return nil })
	}()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	// health.ready answers
	require.Eventually(t, func() bool {
		resp, err := nc.Request("waypoint.dev.module.demo.health.ready", nil, 200*time.Millisecond)
		return err == nil && string(resp.Data) == "ok"
	}, 5*time.Second, 100*time.Millisecond)

	// stats flow and carry a stamp
	statsCh := make(chan *waypointv1.ModuleStats, 1)
	_, err = nc.Subscribe("waypoint.dev.module.demo.stats", func(msg *natsgo.Msg) {
		var st waypointv1.ModuleStats
		if proto.Unmarshal(msg.Data, &st) == nil {
			select {
			case statsCh <- &st:
			default:
			}
		}
	})
	require.NoError(t, err)
	select {
	case st := <-statsCh:
		assert.NotZero(t, st.GetStamp().GetMonoNs())
	case <-time.After(5 * time.Second):
		t.Fatal("no ModuleStats within 5s")
	}

	cancel()
	require.NoError(t, <-done)
}

func TestScopedPublishRejectsForeignSubject(t *testing.T) {
	_, url := startServer(t)
	t.Setenv("WAYPOINT_NATS_URL", url)
	t.Setenv("WAYPOINT_ROVER_ID", "dev")
	t.Setenv("WAYPOINT_MODULE_CREDS", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan error, 1)
	go func() {
		_ = Run(ctx, Options{ID: "demo"}, func(m *M) error {
			got <- m.Publish("waypoint.dev.cmd.drive", nil)
			return nil
		})
	}()
	select {
	case err := <-got:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outside module sandbox")
	case <-time.After(5 * time.Second):
		t.Fatal("setup did not run")
	}
	cancel()
}
