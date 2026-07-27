package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/stretchr/testify/require"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"google.golang.org/protobuf/proto"
)

func TestEpisodeWriterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	specs := []StreamSpec{
		{Subject: "telemetry.drive", Message: "waypoint.v1.DriveTelemetry"},
		{Subject: "cmd.drive", Message: "waypoint.v1.DriveCommand"},
		{Subject: "module.mydrill.drill.state"}, // schemaless generic class
	}
	ew, err := newEpisodeWriter(dir, "ep-test-0001", specs)
	require.NoError(t, err)
	require.NoError(t, ew.addVideoChannel("cam0"))

	msg, err := proto.Marshal(&waypointv1.DriveTelemetry{})
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, ew.write("telemetry.drive", msg, now))
	require.NoError(t, ew.write("telemetry.drive", msg, now.Add(20*time.Millisecond)))
	require.NoError(t, ew.write("module.mydrill.drill.state", []byte{0x08, 0x01}, now))
	require.NoError(t, ew.writeVideo("cam0", []byte{0, 0, 0, 1, 0x65}, now))

	sc := Sidecar{FormatVersion: 1, EpisodeID: "ep-test-0001", TaskLabel: "t"}
	require.NoError(t, ew.finalize(&sc))

	// .partial must be gone, final file + counts present.
	_, err = os.Stat(filepath.Join(dir, "ep-test-0001.mcap.partial"))
	require.True(t, os.IsNotExist(err))
	require.Equal(t, uint64(2), streamCount(sc, "telemetry.drive"))
	require.Equal(t, uint64(1), streamCount(sc, "camera.cam0/h264"))
	require.Equal(t, uint64(1), streamCount(sc, "module.mydrill.drill.state"))
	require.Greater(t, sc.Bytes, uint64(0))

	f, err := os.Open(filepath.Join(dir, "ep-test-0001.mcap"))
	require.NoError(t, err)
	defer f.Close()
	r, err := mcap.NewReader(f)
	require.NoError(t, err)
	info, err := r.Info()
	require.NoError(t, err)
	topics := map[string]bool{}
	schemaIDs := map[string]uint16{}
	for _, ch := range info.Channels {
		topics[ch.Topic] = true
		schemaIDs[ch.Topic] = ch.SchemaID
		require.Equal(t, "protobuf", ch.MessageEncoding)
	}
	require.True(t, topics["telemetry.drive"])
	require.True(t, topics["cmd.drive"])
	require.True(t, topics["camera.cam0/h264"])
	require.True(t, topics["module.mydrill.drill.state"])
	require.Equal(t, uint16(0), schemaIDs["module.mydrill.drill.state"])
	names := map[string]bool{}
	for _, s := range info.Schemas {
		names[s.Name] = true
	}
	require.True(t, names["waypoint.v1.DriveTelemetry"])
	require.True(t, names["foxglove.CompressedVideo"])
}

func streamCount(sc Sidecar, subject string) uint64 {
	for _, s := range sc.Streams {
		if s.Subject == subject {
			return s.Count
		}
	}
	return 0
}
