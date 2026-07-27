package modules

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestDesiredCache_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.desired.pb")
	want := &waypointv1.DesiredModuleSet{
		T: timestamppb.New(time.Unix(1700000000, 0).UTC()),
		Modules: []*waypointv1.ModuleDesired{
			{Id: "power-monitor", Version: "0.1.0", RawUrl: "https://p/x.raw", RawSha256: "abc"},
		},
	}
	require.NoError(t, WriteDesiredCache(path, want))
	got, err := ReadDesiredCache(path)
	require.NoError(t, err)
	require.Len(t, got.Modules, 1)
	require.Equal(t, "power-monitor", got.Modules[0].Id)
}

func TestDesiredCache_Missing(t *testing.T) {
	got, err := ReadDesiredCache(filepath.Join(t.TempDir(), "absent.pb"))
	require.NoError(t, err, "missing cache must yield empty set, not error")
	require.NotNil(t, got)
}

func TestDesiredCache_Corrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pb")
	require.NoError(t, os.WriteFile(path, []byte("not a protobuf"), 0o644))
	_, err := ReadDesiredCache(path)
	require.Error(t, err)
}
