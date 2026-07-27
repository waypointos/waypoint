package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/stretchr/testify/require"
)

func TestRecoverPartials(t *testing.T) {
	dir := t.TempDir()
	// Simulate a crash: write messages but never finalize. Each message exceeds
	// the writer's chunk size so every one flushes a chunk to disk, leaving the
	// partial recoverable (loss is bounded to the last unflushed chunk).
	ew, err := newEpisodeWriter(dir, "ep-crash-0001",
		[]StreamSpec{{Subject: "telemetry.drive", Message: "waypoint.v1.DriveTelemetry"}})
	require.NoError(t, err)
	body := make([]byte, (1<<20)+1)
	for i := 0; i < 3; i++ {
		require.NoError(t, ew.write("telemetry.drive", body, time.Now()))
	}
	ew.cf.f.Close() // abandon without Close/rename, like a power cut

	require.NoError(t, RecoverPartials(dir))

	finalP := filepath.Join(dir, "ep-crash-0001.mcap")
	_, err = os.Stat(finalP)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "ep-crash-0001.mcap.partial"))
	require.True(t, os.IsNotExist(err))
	scs, err := listSidecars(dir)
	require.NoError(t, err)
	require.Len(t, scs, 1)
	require.True(t, scs[0].Crashed)
	require.Nil(t, scs[0].Success)

	// The recovered file must be a valid, footered MCAP that standard tooling
	// can open via the summary index, not just raw salvage.
	f, err := os.Open(finalP)
	require.NoError(t, err)
	defer f.Close()
	rdr, err := mcap.NewReader(f)
	require.NoError(t, err)
	info, err := rdr.Info()
	require.NoError(t, err)
	require.Len(t, info.Channels, 1)
	require.Equal(t, uint64(3), info.Statistics.MessageCount)
}
