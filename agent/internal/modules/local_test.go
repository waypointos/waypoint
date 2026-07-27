package modules

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestMergeDesired_LocalWins(t *testing.T) {
	proxy := &waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "a", Version: "1"}, {Id: "b", Version: "1"}}}
	local := &waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "b", Version: "2"}, {Id: "c", Version: "1"}}}
	got := map[string]string{}
	for _, m := range mergeDesired(proxy, local).Modules {
		got[m.Id] = m.Version
	}
	require.Equal(t, map[string]string{"a": "1", "b": "2", "c": "1"}, got)
}

func TestMergeDesired_NilSafe(t *testing.T) {
	require.Empty(t, mergeDesired(nil, nil).Modules)
	require.Len(t, mergeDesired(&waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "a"}}}, nil).Modules, 1)
	require.Len(t, mergeDesired(nil, &waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "a"}}}).Modules, 1)
}

func TestLocalDesiredStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.local.pb")
	set := &waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "umr", Version: "0.1.0", ConfigToml: "x=1"}}}
	require.NoError(t, WriteLocalDesired(path, set))
	got, err := ReadLocalDesired(path)
	require.NoError(t, err)
	require.Len(t, got.Modules, 1)
	require.Equal(t, "umr", got.Modules[0].Id)
	set2 := upsertModule(got, &waypointv1.ModuleDesired{Id: "pm", Version: "0.2.0"})
	require.Len(t, set2.Modules, 2)
	set2b := upsertModule(set2, &waypointv1.ModuleDesired{Id: "umr", Version: "0.9.9"})
	require.Len(t, set2b.Modules, 2) // upsert replaces, not append
	require.Equal(t, "0.9.9", findVersion(set2b, "umr"))
	require.True(t, containsModule(set2b, "umr"))
	set3 := removeModule(set2b, "umr")
	require.Len(t, set3.Modules, 1)
	require.Equal(t, "pm", set3.Modules[0].Id)
	require.False(t, containsModule(set3, "umr"))
}

func TestTrustStore_PinAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.trust.toml")
	require.NoError(t, CheckPinnedSAN(path, "umr", "anything")) // unseen → ok (TOFU)
	require.NoError(t, PinSAN(path, "umr", "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.1.0"))
	require.NoError(t, CheckPinnedSAN(path, "umr", "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.1.0")) // match → ok
	require.ErrorIs(t, CheckPinnedSAN(path, "umr", "https://github.com/evil/r/.github/workflows/release.yml@refs/tags/v9"), ErrSANMismatch)
	require.NoError(t, CheckPinnedSAN(path, "new", "anything")) // different id, unseen → ok
}

func TestTrustStore_AllowsNewVersionSameWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.trust.toml")
	const v020 = "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.2.0"
	const v021 = "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.2.1"
	const evil = "https://github.com/evil/r/.github/workflows/release.yml@refs/tags/v0.2.1"
	require.NoError(t, PinSAN(path, "umr", v020))
	require.NoError(t, CheckPinnedSAN(path, "umr", v021))         // same workflow, newer tag → ok
	require.ErrorIs(t, CheckPinnedSAN(path, "umr", evil), ErrSANMismatch) // different repo → blocked
}

func TestTrustStore_Overwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.trust.toml")
	require.NoError(t, PinSAN(path, "x", "a"))
	require.NoError(t, CheckPinnedSAN(path, "x", "a"))
	require.NoError(t, PinSAN(path, "x", "b"))
	require.NoError(t, CheckPinnedSAN(path, "x", "b"))
	require.ErrorIs(t, CheckPinnedSAN(path, "x", "a"), ErrSANMismatch)
}

// findVersion returns the Version of the module with the given id, or "".
func findVersion(set *waypointv1.DesiredModuleSet, id string) string {
	for _, m := range set.Modules {
		if m.Id == id {
			return m.Version
		}
	}
	return ""
}
