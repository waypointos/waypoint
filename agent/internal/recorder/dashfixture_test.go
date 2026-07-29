package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"google.golang.org/protobuf/proto"
)

// Regenerates the dashboard's episode-player fixture from the real writer so
// the TS reader is tested against bytes this package actually produces.
// Run: WRITE_DASHBOARD_FIXTURE=1 go test ./internal/recorder/ -run TestWriteDashboardFixture
func TestWriteDashboardFixture(t *testing.T) {
	if os.Getenv("WRITE_DASHBOARD_FIXTURE") == "" {
		t.Skip("set WRITE_DASHBOARD_FIXTURE=1 to regenerate")
	}
	dir := t.TempDir()
	specs := []StreamSpec{
		{Subject: "telemetry.motors", Message: "waypoint.v1.MotorTelemetry"},
		{Subject: "module.drill.sensor.state", Message: "waypoint.v1.SensorReadings"},
		{Subject: "module.drill.drill.state"},
	}
	ew, err := newEpisodeWriter(dir, "fixture", specs)
	require.NoError(t, err)
	require.NoError(t, ew.addVideoChannel("cam0"))

	// MotorTelemetry is one message per servo, so the plan's "first motor" is
	// id=1 and its numeric series is velocity_radps.
	motor := func(speed float64) []byte {
		b, err := proto.Marshal(&waypointv1.MotorTelemetry{
			Id:            1,
			VelocityRadps: proto.Float64(speed),
		})
		require.NoError(t, err)
		return b
	}
	cell := func(name string, val *float64) *waypointv1.SensorReading {
		r := &waypointv1.SensorReading{Name: name, Unit: "g", Ok: val != nil}
		r.Value = val
		return r
	}
	f := func(v float64) *float64 { return &v }
	sensors := func(a float64, b *float64) []byte {
		bts, err := proto.Marshal(&waypointv1.SensorReadings{
			Readings: []*waypointv1.SensorReading{cell("cell_a_g", f(a)), cell("cell_b_g", b)},
		})
		require.NoError(t, err)
		return bts
	}

	at := func(s int64) time.Time { return time.Unix(s, 0) }
	require.NoError(t, ew.write("telemetry.motors", motor(0.5), at(1)))
	require.NoError(t, ew.write("telemetry.motors", motor(0.6), at(2)))
	require.NoError(t, ew.write("telemetry.motors", motor(0.7), at(3)))
	require.NoError(t, ew.write("module.drill.sensor.state", sensors(100, f(200)), at(1)))
	require.NoError(t, ew.write("module.drill.sensor.state", sensors(110, nil), at(2)))
	require.NoError(t, ew.write("module.drill.sensor.state", sensors(120, f(220)), at(3)))
	require.NoError(t, ew.write("module.drill.drill.state", []byte{0xde, 0xad}, at(1)))
	require.NoError(t, ew.write("module.drill.drill.state", []byte{0xde, 0xad}, at(2)))
	require.NoError(t, ew.writeVideo("cam0", []byte{0, 0, 0, 1, 0x67, 0x42, 0, 0x1e}, at(1)))
	require.NoError(t, ew.writeVideo("cam0", []byte{0, 0, 0, 1, 0x41}, at(2)))

	sc := &Sidecar{FormatVersion: FormatVersion, EpisodeID: "fixture", TaskLabel: "fixture"}
	require.NoError(t, ew.finalize(sc))

	dest := filepath.Join("..", "..", "..", "dashboard", "src", "lib", "episode", "__fixtures__", "episode.mcap")
	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	data, err := os.ReadFile(filepath.Join(dir, "fixture.mcap"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dest, data, 0o644))
}
