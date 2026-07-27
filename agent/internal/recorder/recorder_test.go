package recorder

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	natssrv "github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"google.golang.org/protobuf/proto"
)

func startEmbeddedNATS(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := natssrv.StartEmbedded(natssrv.Options{Port: -1})
	require.NoError(t, err)
	t.Cleanup(srv.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, srv.WaitReady(ctx))
	nc, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}

func newTestRecorder(t *testing.T, nc *nats.Conn, dir string) *Recorder {
	t.Helper()
	r := New(Config{
		NC:      nc,
		RoverID: "dev",
		Dir:     dir,
		Specs: func() []StreamSpec {
			return []StreamSpec{{Subject: "telemetry.drive", Message: "waypoint.v1.DriveTelemetry"}}
		},
		Tick: 50 * time.Millisecond,
	})
	require.NoError(t, r.Serve())
	// Stop any lingering episode so its background goroutines exit before a
	// later test mutates the package-global freeBytes.
	t.Cleanup(func() { _, _ = r.StopEpisode(false, "") })
	return r
}

func TestStartCaptureStop(t *testing.T) {
	nc := startEmbeddedNATS(t)
	dir := t.TempDir()
	_ = newTestRecorder(t, nc, dir)

	var startResp waypointv1.RecorderStartResponse
	req, _ := proto.Marshal(&waypointv1.RecorderStartRequest{TaskLabel: "push block"})
	msg, err := nc.Request("waypoint.dev.rpc.recorder_start", req, 2*time.Second)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(msg.Data, &startResp))
	require.True(t, startResp.Ok, startResp.Reason)
	require.NotEmpty(t, startResp.EpisodeId)

	body, _ := proto.Marshal(&waypointv1.DriveTelemetry{})
	for i := 0; i < 5; i++ {
		require.NoError(t, nc.Publish("waypoint.dev.telemetry.drive", body))
	}
	nc.Flush()
	time.Sleep(200 * time.Millisecond) // let async handlers write

	var stopResp waypointv1.RecorderStopResponse
	sreq, _ := proto.Marshal(&waypointv1.RecorderStopRequest{Success: true, Notes: "n"})
	msg, err = nc.Request("waypoint.dev.rpc.recorder_stop", sreq, 2*time.Second)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(msg.Data, &stopResp))
	require.True(t, stopResp.Ok)
	require.NotNil(t, stopResp.Summary.Success)
	require.True(t, *stopResp.Summary.Success)
	require.Equal(t, uint64(5), stopResp.Summary.Streams[0].Count)

	_, err = os.Stat(filepath.Join(dir, startResp.EpisodeId+".mcap"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, startResp.EpisodeId+".json"))
	require.NoError(t, err)

	// episode_list returns the finished episode.
	var list waypointv1.EpisodeList
	lreq, _ := proto.Marshal(&waypointv1.EpisodeListRequest{})
	msg, err = nc.Request("waypoint.dev.rpc.episode_list", lreq, 2*time.Second)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(msg.Data, &list))
	require.Len(t, list.Episodes, 1)
	require.Equal(t, "push block", list.Episodes[0].TaskLabel)
}

func TestStartRefusedWhileRecording(t *testing.T) {
	nc := startEmbeddedNATS(t)
	r := newTestRecorder(t, nc, t.TempDir())
	_, err := r.StartEpisode("one")
	require.NoError(t, err)
	_, err = r.StartEpisode("two")
	require.ErrorContains(t, err, "already recording")
}

func TestStartRefusedOnLowDisk(t *testing.T) {
	nc := startEmbeddedNATS(t)
	old := freeBytes
	freeBytes = func(string) (uint64, error) { return 1 << 20, nil } // 1 MiB
	t.Cleanup(func() { freeBytes = old })
	r := newTestRecorder(t, nc, t.TempDir())
	_, err := r.StartEpisode("x")
	require.ErrorContains(t, err, "low disk")
}

func TestAutoStopOnDiskPressure(t *testing.T) {
	nc := startEmbeddedNATS(t)
	// Atomic so the watch goroutine and the test goroutine share free safely.
	var free atomic.Uint64
	free.Store(10 << 30)
	old := freeBytes
	freeBytes = func(string) (uint64, error) { return free.Load(), nil }
	t.Cleanup(func() { freeBytes = old })

	dir := t.TempDir()
	stopped := make(chan string, 1)
	r := New(Config{
		NC: nc, RoverID: "dev", Dir: dir,
		Specs:      func() []StreamSpec { return nil },
		Tick:       20 * time.Millisecond,
		OnAutoStop: func(reason string) { stopped <- reason },
	})
	require.NoError(t, r.Serve())
	id, err := r.StartEpisode("x")
	require.NoError(t, err)
	free.Store(1 << 20) // drop below the default threshold mid-episode

	select {
	case reason := <-stopped:
		require.Contains(t, reason, "low disk")
	case <-time.After(2 * time.Second):
		t.Fatal("auto-stop never fired")
	}
	scs, err := listSidecars(dir)
	require.NoError(t, err)
	require.Len(t, scs, 1)
	require.Equal(t, id, scs[0].EpisodeID)
	require.Nil(t, scs[0].Success)
	require.Contains(t, scs[0].Notes, "auto-stopped")
}

func TestRecorderEventPublished(t *testing.T) {
	nc := startEmbeddedNATS(t)
	events := make(chan *waypointv1.RecorderEvent, 16)
	_, err := nc.Subscribe("waypoint.dev.event.recorder", func(m *nats.Msg) {
		var ev waypointv1.RecorderEvent
		if proto.Unmarshal(m.Data, &ev) == nil {
			events <- &ev
		}
	})
	require.NoError(t, err)
	r := newTestRecorder(t, nc, t.TempDir())
	_, err = r.StartEpisode("x")
	require.NoError(t, err)
	ev := waitState(t, events, waypointv1.RecorderState_RECORDER_STATE_RECORDING)
	require.NotEmpty(t, ev.EpisodeId)
	_, err = r.StopEpisode(true, "")
	require.NoError(t, err)
	waitState(t, events, waypointv1.RecorderState_RECORDER_STATE_IDLE)
}

func waitState(t *testing.T, ch chan *waypointv1.RecorderEvent, want waypointv1.RecorderState) *waypointv1.RecorderEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.State == want {
				return ev
			}
		case <-deadline:
			t.Fatalf("never saw state %v", want)
		}
	}
}

func TestPathAndDeleteGuards(t *testing.T) {
	nc := startEmbeddedNATS(t)
	dir := t.TempDir()
	r := newTestRecorder(t, nc, dir)

	// Traversal and malformed ids are rejected before touching the filesystem.
	for _, id := range []string{"", "..", "../etc/passwd", "a/b"} {
		_, err := r.Path(id)
		require.Error(t, err, "id %q must be rejected", id)
	}

	// The in-flight episode can be neither downloaded nor deleted.
	id, err := r.StartEpisode("guard")
	require.NoError(t, err)
	_, err = r.Path(id)
	require.ErrorContains(t, err, "recording")
	require.ErrorContains(t, r.Delete(id), "recording")

	// After a clean stop both succeed.
	_, err = r.StopEpisode(true, "")
	require.NoError(t, err)
	p, err := r.Path(id)
	require.NoError(t, err)
	require.FileExists(t, p)
	require.NoError(t, r.Delete(id))
	_, err = r.Path(id)
	require.Error(t, err, "deleted episode must not resolve")
}
