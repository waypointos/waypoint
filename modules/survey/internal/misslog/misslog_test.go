package misslog

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerWritesRows(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l, path, err := New(dir)
	require.NoError(t, err)
	defer l.Close()

	ts := time.Unix(1700000000, 500_000_000)
	require.NoError(t, l.Row(ts, 1.2345, -2.5, 90.04, 0.35, "TRANSIT", -1, ""))
	require.NoError(t, l.Row(ts, 4.0, 0.0, 0.0, 0.0, "SENSE", 17, "detected wp=0 id=17"))

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)
	require.Len(t, recs, 3)
	assert.Equal(t, []string{"t", "x", "y", "heading_deg", "speed_mps", "state", "detected_id", "event"}, recs[0])
	assert.Equal(t, []string{"1700000000.500", "1.234", "-2.500", "90.0", "0.350", "TRANSIT", "", ""}, recs[1])
	assert.Equal(t, "17", recs[2][6])
	assert.Equal(t, "SENSE", recs[2][5])
}

func TestLoggerCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	l, path, err := New(dir)
	require.NoError(t, err)
	l.Close()
	assert.FileExists(t, path)
}
