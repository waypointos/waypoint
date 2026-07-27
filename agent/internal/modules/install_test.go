package modules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// fakeVerifier returns a fixed SAN without touching Sigstore.
type fakeVerifier struct{ san string }

func (f fakeVerifier) Verify(_ context.Context, _, _ []byte, _ string) (string, error) {
	return f.san, nil
}

// installFakePortable seeds a minimal module.toml under the mount path and is
// otherwise a no-op. Named distinctly from fakePortable in reconciler_test.go.
type installFakePortable struct{}

const minimalModuleTOML = `name = "umr"
label = "UMR"
version = "0.1.0"
entrypoint = "waypoint-module-umr"
[ui.static]
tab_id = "m-umr"
bundle = "/dashboard/panel.js"
`

func (installFakePortable) Attach(_ context.Context, _ string) error { return nil }
func (installFakePortable) Detach(_ context.Context, _ string) error { return nil }

func (installFakePortable) Mount(_ context.Context, _, mountPath string) error {
	dir := filepath.Join(mountPath, "usr/share/waypoint/modules/umr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "module.toml"), []byte(minimalModuleTOML), 0o644)
}

func (installFakePortable) Unmount(_ context.Context, _ string) error { return nil }

// newTestManager returns a Manager wired for install tests. reconcileFn is a
// no-op so tests don't invoke portablectl or require a running agent.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	opts := Options{
		ConfigDirPath: filepath.Join(base, "config"),
		RawStorageDir: filepath.Join(base, "raw"),
		RuntimeDir:    filepath.Join(base, "run"),
	}
	for _, d := range []string{opts.ConfigDirPath, opts.RawStorageDir, opts.RuntimeDir} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	m := &Manager{
		opts:          opts,
		hw:            map[string]*HealthWatcher{},
		healthPollers: map[string]context.CancelFunc{},
		verifier:      fakeVerifier{san: "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.1.0"},
		portable:      installFakePortable{},
	}
	m.reconcileFn = func(_ context.Context, _ *waypointv1.DesiredModuleSet) error { return nil }
	return m
}

func TestInstall_StageConfirmUninstall(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	// Stage
	preview, err := m.StageModule(ctx, []byte("RAW"), []byte("BUNDLE"))
	require.NoError(t, err)
	require.Equal(t, "umr", preview.ID)
	require.Equal(t, "0.1.0", preview.Version)
	require.True(t, strings.Contains(preview.SignerSAN, "release.yml@refs/tags/v"),
		"unexpected SAN: %s", preview.SignerSAN)
	// A module with no [hardware] must still yield non-nil slices so the JSON
	// encodes [] (not null) and the dashboard's array-length checks don't crash.
	require.NotNil(t, preview.Devices)
	require.NotNil(t, preview.Permissions)

	// Confirm
	err = m.ConfirmModule(ctx, preview.StageID, "[modules_config.umr]\npassword=\"x\"\n")
	require.NoError(t, err)

	dest := rawDestPath(m.opts.RawStorageDir, "umr", "0.1.0")
	_, statErr := os.Stat(dest)
	require.NoError(t, statErr, "raw blob must exist at dest path")

	require.True(t, containsModule(m.localDesired, "umr"), "local desired must contain umr")

	require.NoError(t, CheckPinnedSAN(m.trustPath(), "umr", preview.SignerSAN))

	configPath := filepath.Join(m.opts.RuntimeDir, "umr", "config.toml")
	_, statErr = os.Stat(configPath)
	require.NoError(t, statErr, "config.toml must exist")

	data, err := os.ReadFile(filepath.Join(m.opts.RuntimeDir, "umr", "config.toml"))
	require.NoError(t, err)
	require.Contains(t, string(data), `password = "x"`)

	// Uninstall
	err = m.UninstallModule(ctx, "umr")
	require.NoError(t, err)
	require.False(t, containsModule(m.localDesired, "umr"), "local desired must not contain umr after uninstall")

	_, statErr = os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "raw blob dir must be removed after uninstall")
}

func TestInstall_StageConfirm_SANMismatch(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	const sanOurs = "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.1.0"
	const sanEvil = "https://github.com/evil/r/.github/workflows/release.yml@refs/tags/v0.1.0"

	// First install from our workflow — pins its identity.
	m.verifier = fakeVerifier{san: sanOurs}
	preview1, err := m.StageModule(ctx, []byte("RAW"), []byte("BUNDLE"))
	require.NoError(t, err)
	require.NoError(t, m.ConfirmModule(ctx, preview1.StageID, ""))

	// Stage a second image signed by a DIFFERENT repo's workflow.
	m.verifier = fakeVerifier{san: sanEvil}
	preview2, err := m.StageModule(ctx, []byte("RAW2"), []byte("BUNDLE2"))
	require.NoError(t, err)

	// Confirm must reject: the pinned workflow identity differs.
	err = m.ConfirmModule(ctx, preview2.StageID, "")
	require.ErrorIs(t, err, ErrSANMismatch)
}

// A new release from the same workflow (newer tag → different SAN ref) must
// confirm cleanly; the pin is on the workflow identity, not the version tag.
func TestInstall_StageConfirm_AllowsSameWorkflowUpgrade(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)

	const v010 = "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.1.0"
	const v020 = "https://github.com/o/r/.github/workflows/release.yml@refs/tags/v0.2.0"

	m.verifier = fakeVerifier{san: v010}
	p1, err := m.StageModule(ctx, []byte("RAW"), []byte("BUNDLE"))
	require.NoError(t, err)
	require.NoError(t, m.ConfirmModule(ctx, p1.StageID, ""))

	m.verifier = fakeVerifier{san: v020}
	p2, err := m.StageModule(ctx, []byte("RAW2"), []byte("BUNDLE2"))
	require.NoError(t, err)
	require.NoError(t, m.ConfirmModule(ctx, p2.StageID, ""), "same-workflow upgrade must be allowed")
}
