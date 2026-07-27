package harness

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceRecordsAndQueries(t *testing.T) {
	tr := newTrace("rov-1")
	tr.record("waypoint.rov-1.telemetry.drive", []byte{1})
	tr.record("waypoint.rov-1.event.mode", []byte{2})
	tr.record("waypoint.rov-1.telemetry.drive", []byte{3})

	drive := tr.Messages("telemetry.drive")
	require.Len(t, drive, 2)
	assert.Equal(t, []byte{1}, drive[0].Data)

	got, err := tr.WaitFor("event.mode", func(m Msg) bool { return m.Data[0] == 2 }, 100*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, []byte{2}, got.Data)

	_, err = tr.WaitFor("event.fault", func(Msg) bool { return true }, 50*time.Millisecond)
	assert.Error(t, err)

	tr.Clear()
	assert.Empty(t, tr.Messages("telemetry.drive"))
}
