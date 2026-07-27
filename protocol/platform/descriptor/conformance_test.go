package descriptor

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type golden struct {
	JointNameToBusID    map[string]uint32 `json:"joint_name_to_bus_id"`
	PlatformOwnedBusIDs []uint32          `json:"platform_owned_bus_ids"`
	WheelSpeeds         []struct {
		VxMps    float64            `json:"vx_mps"`
		WzRadps  float64            `json:"wz_radps"`
		Expected map[string]float64 `json:"expected"`
	} `json:"wheel_speeds"`
}

// fixtures is the platform matrix every conformance check runs over: each
// descriptor paired with its derived golden values.
var fixtures = []struct {
	toml   string
	golden string
}{
	{"../waypoint-rover.toml", "../testdata/waypoint-rover.derived.golden.json"},
	{"../waypoint-bench.toml", "../testdata/waypoint-bench.derived.golden.json"},
}

func TestCanonicalDescriptorConformance(t *testing.T) {
	for _, fx := range fixtures {
		d, err := Load(fx.toml)
		require.NoError(t, err, fx.toml)
		t.Run(d.Platform.ID, func(t *testing.T) {
			raw, err := os.ReadFile(fx.golden)
			require.NoError(t, err)
			var g golden
			require.NoError(t, json.Unmarshal(raw, &g))
			for name, want := range g.JointNameToBusID {
				got, ok := d.BusIDFor(name)
				require.True(t, ok, "joint %s missing", name)
				assert.Equal(t, want, got, "joint %s", name)
			}
			assert.Len(t, d.Joints, len(g.JointNameToBusID))
			assert.ElementsMatch(t, g.PlatformOwnedBusIDs, d.PlatformOwnedBusIDs())
			for _, c := range g.WheelSpeeds {
				got, err := d.WheelSpeeds(c.VxMps, c.WzRadps)
				require.NoError(t, err)
				require.Len(t, got, len(c.Expected))
				for name, want := range c.Expected {
					assert.InDelta(t, want, got[name], 1e-9, "vx=%v wz=%v joint=%s", c.VxMps, c.WzRadps, name)
				}
			}
		})
	}
}

// Every observation stream subject must exist in protocol/subjects.toml. This
// is the registry drift rule applied to the descriptor.
func TestObservationSubjectsRegistered(t *testing.T) {
	raw, err := os.ReadFile("../../subjects.toml")
	require.NoError(t, err)
	var tables map[string]map[string]string
	_, err = toml.Decode(string(raw), &tables)
	require.NoError(t, err)
	known := map[string]bool{}
	for _, table := range tables {
		for leaf := range table {
			known[leaf] = true
		}
	}
	for _, fx := range fixtures {
		d, err := Load(fx.toml)
		require.NoError(t, err, fx.toml)
		t.Run(d.Platform.ID, func(t *testing.T) {
			require.NotNil(t, d.Observations)
			for _, s := range d.Observations.Streams {
				assert.True(t, known[s.Subject], "observation subject %q not in subjects.toml", s.Subject)
			}
		})
	}
}
