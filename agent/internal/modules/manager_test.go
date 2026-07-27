package modules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/localauth"
	"github.com/waypointos/waypoint/agent/internal/nats"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestManager_EndToEnd(t *testing.T) {
	installDir := t.TempDir()
	configDir := t.TempDir()
	runtimeDir := t.TempDir()

	mustWrite(t, filepath.Join(installDir, "umr", "module.toml"), sampleManifestTOML)
	mustWrite(t, filepath.Join(configDir, "modules.toml"), `
[[modules]]
id = "umr"
enable = true

[modules_config.umr]
host = "https://192.168.105.1"
password = "x"
`)

	builder := localauth.NewBuilder()

	// Create manager FIRST (it calls AddModuleUser, which mutates the builder),
	// then start the server with the resulting users + writes creds to runtimeDir.
	mgr, err := New(Options{
		InstallDir: installDir,
		ConfigPath: filepath.Join(configDir, "modules.toml"),
		RuntimeDir: runtimeDir,
		Auth:       builder,
		Supervisor: NewNoopSupervisor(),
		RoverID:    "rover-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	users, defaultUser := builder.Build()
	srv, err := nats.StartEmbedded(nats.Options{Port: -1, Users: users, NoAuthUser: defaultUser})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	cli, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	mgr.SetConn(cli)

	// Fake module: replies to health.ready under module-umr creds.
	go func() {
		// Wait for manager to register the module user, then connect.
		time.Sleep(150 * time.Millisecond)
		username, password := readCredsEnv(t, filepath.Join(runtimeDir, "umr", "creds.env"))
		mc, err := natsgo.Connect(srv.ClientURL(), natsgo.UserInfo(username, password))
		if err != nil {
			t.Logf("module connect: %v", err)
			return
		}
		_, _ = mc.Subscribe("waypoint.rover-test.module.umr.health.ready", func(m *natsgo.Msg) {
			_ = m.Respond([]byte("ok"))
		})
		<-ctx.Done()
		mc.Close()
	}()

	var mu sync.Mutex
	var observed int
	if _, err := cli.Subscribe("waypoint.rover-test.infra.modules", func(m *natsgo.Msg) {
		mu.Lock()
		observed++
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}

	go mgr.Run(ctx)
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if observed < 2 {
		t.Fatalf("want ≥2 infra.modules publishes, got %d", observed)
	}
}

func TestManager_HandlesDesiredPush(t *testing.T) {
	called := make(chan *waypointv1.DesiredModuleSet, 1)
	m := &Manager{
		opts: Options{RoverID: "rover-test", ConfigDirPath: t.TempDir()},
		reconcileFn: func(ctx context.Context, set *waypointv1.DesiredModuleSet) error {
			called <- set
			return nil
		},
	}
	bytes, _ := proto.Marshal(&waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{{Id: "x", Version: "0.1.0"}},
	})
	m.handleDesired(context.Background(), &natsgo.Msg{Data: bytes})
	select {
	case got := <-called:
		require.Equal(t, "x", got.Modules[0].Id)
	case <-time.After(time.Second):
		t.Fatal("Reconcile not invoked")
	}
}

func TestManager_AppliesDevOverrideOnDevImage(t *testing.T) {
	dir := t.TempDir()
	overridePath := filepath.Join(dir, "modules.desired.override.toml")
	require.NoError(t, os.WriteFile(overridePath, []byte(`
[[modules]]
id        = "x"
version   = "0.0.1-dev"
raw_path  = "/data/waypoint/modules/x/local.raw"
`), 0o644))

	m := &Manager{
		opts: Options{RoverID: "rover-test", IsDevImage: true, ConfigDirPath: dir},
	}
	require.NoError(t, m.loadDevOverrideIntoLocal())
	require.True(t, containsModule(m.localDesired, "x"), "override module must be in local desired")
	require.Equal(t, "file:///data/waypoint/modules/x/local.raw", m.localDesired.Modules[0].RawUrl)
}

// TestManager_DevOverrideMergesWithProxyDesired asserts the latent bug is fixed:
// the dev override must not detach proxy modules (both must appear in the reconcile).
func TestManager_DevOverrideMergesWithProxyDesired(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules.desired.override.toml"), []byte(`
[[modules]]
id        = "x"
version   = "0.0.1-dev"
raw_path  = "/data/waypoint/modules/x/local.raw"
`), 0o644))

	called := make(chan *waypointv1.DesiredModuleSet, 1)
	m := &Manager{
		opts: Options{RoverID: "rover-test", IsDevImage: true, ConfigDirPath: dir},
		reconcileFn: func(_ context.Context, set *waypointv1.DesiredModuleSet) error {
			select {
			case called <- set:
			default:
			}
			return nil
		},
		proxyDesired: &waypointv1.DesiredModuleSet{
			Modules: []*waypointv1.ModuleDesired{{Id: "proxy-mod", Version: "1.0.0"}},
		},
	}
	require.NoError(t, m.loadDevOverrideIntoLocal())
	require.NoError(t, m.reconcileMerged(context.Background()))
	select {
	case got := <-called:
		ids := map[string]bool{}
		for _, mod := range got.Modules {
			ids[mod.Id] = true
		}
		require.True(t, ids["x"], "override module must be in merged reconcile")
		require.True(t, ids["proxy-mod"], "proxy module must not be detached by override")
	case <-time.After(time.Second):
		t.Fatal("reconcile not invoked")
	}
}

func TestManager_IgnoresOverrideOnProdImage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules.desired.override.toml"),
		[]byte("[[modules]]\nid=\"x\"\nversion=\"0.0.1\"\nraw_path=\"/x.raw\"\n"), 0o644))
	m := &Manager{
		opts: Options{IsDevImage: false, ConfigDirPath: dir},
	}
	require.NoError(t, m.loadDevOverrideIntoLocal())
	require.Nil(t, m.localDesired, "prod image must not populate local desired from override file")
}

func TestManager_NoOverrideFileIsNoop(t *testing.T) {
	// Dev image with no override file present must succeed silently.
	dir := t.TempDir()
	m := &Manager{
		opts: Options{IsDevImage: true, ConfigDirPath: dir},
	}
	require.NoError(t, m.loadDevOverrideIntoLocal())
	require.Nil(t, m.localDesired)
}

func TestManager_RuntimeAttachRegistersAndPollsHealth(t *testing.T) {
	pub := NewSnapshotPublisher(nil, "rover-1", nil)
	sup := &fakeSupervisor{active: true}
	mgr := &Manager{opts: Options{Supervisor: sup, RoverID: "rover-1"}, pub: pub,
		hw: map[string]*HealthWatcher{}, healthPollers: map[string]context.CancelFunc{}}

	m := &Manifest{Name: "power-monitor", Label: "Power Monitor", Version: "0.1.0",
		UI: UIBinding{Kind: UIKindProxy, TabID: "m-power-monitor", ProxyPort: 8090, LANOnly: true}}
	mgr.onModuleAttached(context.Background(), "power-monitor", m)

	require.Eventually(t, func() bool {
		snap := pub.buildSnapshot()
		return len(snap.Modules) == 1 && snap.Modules[0].Healthy
	}, time.Second, 10*time.Millisecond)

	mgr.onModuleDetached("power-monitor")
	require.Empty(t, pub.buildSnapshot().Modules)
	require.Empty(t, mgr.healthPollers, "detach must cancel + remove the health poller")
}

func TestProvisionCachedDesiredCreds(t *testing.T) {
	root := t.TempDir()
	runtime := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	cachePath := filepath.Join(cacheDir, "modules.desired.pb")
	require.NoError(t, WriteDesiredCache(cachePath, &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{{Id: "umr", Version: "0.1.0"}},
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "umr", "0.1.0"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "umr", "0.1.0", "umr.raw"), []byte("x"), 0o644))

	b := localauth.NewBuilder()
	ids := provisionCachedDesiredCreds(cachePath, root, runtime, "rover-1", b, nil)
	require.Equal(t, []string{"umr"}, ids)
	users, _ := b.Build()
	require.GreaterOrEqual(t, len(users), 2) // default user + the module user
}

// TestNew_InstalledModuleCredsWinOverStaleBakedSpec reproduces the on-rover bug
// where an OLD image still bakes a module (narrow, stale publish perms) that is
// also installed at runtime with a newer manifest. The installed copy's creds
// (the full module.<id>.> subtree) must win, or the module's new publishes
// (e.g. module.umr.uplink) are denied with a NATS Publish Violation.
func TestNew_InstalledModuleCredsWinOverStaleBakedSpec(t *testing.T) {
	installDir := t.TempDir()
	configDir := t.TempDir()
	runtimeDir := t.TempDir()
	rawDir := t.TempDir()

	// Baked umr manifest with a STALE narrow publish list (stats only, no uplink).
	umrDir := filepath.Join(installDir, "umr")
	require.NoError(t, os.MkdirAll(umrDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(umrDir, "module.toml"),
		[]byte("name = \"umr\"\nentrypoint = \"x\"\n[permissions]\npublish = [\"waypoint.*.module.umr.stats\"]\n"), 0o644))
	// Enable it so Discover surfaces the baked spec.
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "modules.toml"),
		[]byte("[[modules]]\nid = \"umr\"\nenable = true\n"), 0o644))
	// Installed copy in the LOCAL desired store, with its .raw on disk.
	require.NoError(t, WriteLocalDesired(filepath.Join(configDir, "modules.local.pb"),
		&waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "umr", Version: "0.2.0"}}}))
	require.NoError(t, os.MkdirAll(filepath.Join(rawDir, "modules", "umr", "0.2.0"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rawDir, "modules", "umr", "0.2.0", "umr.raw"), []byte("x"), 0o644))

	builder := localauth.NewBuilder()
	_, err := New(Options{
		InstallDir:    installDir,
		ConfigPath:    filepath.Join(configDir, "modules.toml"),
		ConfigDirPath: configDir,
		RawStorageDir: rawDir,
		RuntimeDir:    runtimeDir,
		Auth:          builder,
		Supervisor:    NewNoopSupervisor(),
		RoverID:       "rover-1",
	})
	require.NoError(t, err)

	users, _ := builder.Build()
	var umr *natsserver.User
	for _, u := range users {
		if u.Username == "module-umr" {
			umr = u
			break
		}
	}
	require.NotNil(t, umr, "module-umr user must exist")
	// The installed copy's wildcard grant wins → the whole subtree (incl. uplink)
	// is allowed; the stale baked stats-only perm must NOT be what got applied.
	require.Contains(t, umr.Permissions.Publish.Allow, "waypoint.rover-1.module.umr.>")
	require.NotContains(t, umr.Permissions.Publish.Allow, "waypoint.rover-1.module.umr.stats")
}

// TestProvisionCachedDesiredCreds_WritesConfigFromDesired verifies an
// install-only module gets its runtime config.toml at boot from the desired
// entry's ConfigToml. /run is tmpfs, so this is the only thing that restores a
// module's per-rover settings (e.g. a router host) after a reboot; without it a
// module no longer baked into the image boots with an empty config.
func TestProvisionCachedDesiredCreds_WritesConfigFromDesired(t *testing.T) {
	root := t.TempDir()
	runtime := t.TempDir()
	cachePath := filepath.Join(root, "modules.local.pb")
	require.NoError(t, WriteLocalDesired(cachePath, &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{{
			Id:         "umr",
			Version:    "0.1.0",
			ConfigToml: "[modules_config.umr]\nhost = \"https://192.168.105.1\"\n",
		}},
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "umr", "0.1.0"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "umr", "0.1.0", "umr.raw"), []byte("x"), 0o644))

	b := localauth.NewBuilder()
	ids := provisionCachedDesiredCreds(cachePath, root, runtime, "rover-1", b, nil)
	require.Equal(t, []string{"umr"}, ids)

	data, err := os.ReadFile(filepath.Join(runtime, "umr", "config.toml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "192.168.105.1", "config.toml must carry the module's per-rover settings")
}

// writeDesiredConfig is what a live config edit rides: it must unwrap
// [modules_config.<id>] into the flat config.toml the module reads, without
// needing the module's NATS creds (which it already holds by then).
func TestWriteDesiredConfig_UnwrapsModuleTable(t *testing.T) {
	runtime := t.TempDir()
	m := &Manager{opts: Options{RuntimeDir: runtime}}
	d := &waypointv1.ModuleDesired{
		Id:         "umr",
		Version:    "0.4.0",
		ConfigToml: "[modules_config.umr]\nhost = \"https://192.168.105.1\"\npassword = \"secret\"\n",
	}

	require.NoError(t, m.writeDesiredConfig(context.Background(), d))

	data, err := os.ReadFile(filepath.Join(runtime, "umr", "config.toml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "192.168.105.1")
	require.Contains(t, string(data), "secret")
	require.NotContains(t, string(data), "modules_config", "the table wrapper is the agent's, not the module's")
}

// TestProvisionCachedDesiredCreds_FallsBackToRoverConfig verifies that when the
// desired entry carries no ConfigToml (config still lives in modules.toml), the
// module's config.toml is written from the rover's modules.toml config. This
// keeps an already-installed module working after the baked manifest is removed,
// with no re-install required.
func TestProvisionCachedDesiredCreds_FallsBackToRoverConfig(t *testing.T) {
	root := t.TempDir()
	runtime := t.TempDir()
	cachePath := filepath.Join(root, "modules.local.pb")
	require.NoError(t, WriteLocalDesired(cachePath, &waypointv1.DesiredModuleSet{
		Modules: []*waypointv1.ModuleDesired{{Id: "umr", Version: "0.1.0"}},
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "umr", "0.1.0"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "umr", "0.1.0", "umr.raw"), []byte("x"), 0o644))

	b := localauth.NewBuilder()
	roverConfig := map[string]map[string]any{"umr": {"host": "https://192.168.105.1"}}
	ids := provisionCachedDesiredCreds(cachePath, root, runtime, "rover-1", b, roverConfig)
	require.Equal(t, []string{"umr"}, ids)

	data, err := os.ReadFile(filepath.Join(runtime, "umr", "config.toml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "192.168.105.1", "config.toml must fall back to modules.toml settings")
}

// TestNew_InstallOnlyModuleGetsFullCredsAndConfig is the post-baked-removal
// reality: the module is no longer baked into the image (empty InstallDir),
// only installed at runtime (modules.local.pb) with its per-rover config still
// in modules.toml. New() must mint the full module.<id>.> subtree AND restore
// the module's config.toml from modules.toml — otherwise the module boots
// either unable to publish or with no settings to reach its hardware.
func TestNew_InstallOnlyModuleGetsFullCredsAndConfig(t *testing.T) {
	installDir := t.TempDir() // empty: nothing baked
	configDir := t.TempDir()
	runtimeDir := t.TempDir()
	rawDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(configDir, "modules.toml"),
		[]byte("[[modules]]\nid = \"umr\"\nenable = true\n\n[modules_config.umr]\nhost = \"https://192.168.105.1\"\n"), 0o644))
	require.NoError(t, WriteLocalDesired(filepath.Join(configDir, "modules.local.pb"),
		&waypointv1.DesiredModuleSet{Modules: []*waypointv1.ModuleDesired{{Id: "umr", Version: "0.1.0"}}}))
	require.NoError(t, os.MkdirAll(filepath.Join(rawDir, "modules", "umr", "0.1.0"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rawDir, "modules", "umr", "0.1.0", "umr.raw"), []byte("x"), 0o644))

	builder := localauth.NewBuilder()
	_, err := New(Options{
		InstallDir:    installDir,
		ConfigPath:    filepath.Join(configDir, "modules.toml"),
		ConfigDirPath: configDir,
		RawStorageDir: rawDir,
		RuntimeDir:    runtimeDir,
		Auth:          builder,
		Supervisor:    NewNoopSupervisor(),
		RoverID:       "rover-1",
	})
	require.NoError(t, err)

	users, _ := builder.Build()
	var umr *natsserver.User
	for _, u := range users {
		if u.Username == "module-umr" {
			umr = u
			break
		}
	}
	require.NotNil(t, umr, "install-only module must still get a NATS user")
	require.Contains(t, umr.Permissions.Publish.Allow, "waypoint.rover-1.module.umr.>")

	data, err := os.ReadFile(filepath.Join(runtimeDir, "umr", "config.toml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "192.168.105.1", "config must survive baked-manifest removal via modules.toml")
}

// TestManager_EnsureModuleCredsHotGrants proves a module installed at runtime is
// granted its NATS creds live — no reboot. ensureModuleCreds mints the full
// module.<id>.> subtree, writes creds.env + config.toml, and hot-reloads the
// embedded server so the module can immediately publish module.<id>.uplink (the
// subject that was denied on the rover).
func TestManager_EnsureModuleCredsHotGrants(t *testing.T) {
	runtimeDir := t.TempDir()
	builder := localauth.NewBuilder()
	users, def := builder.Build()
	srv, err := nats.StartEmbedded(nats.Options{Port: -1, Users: users, NoAuthUser: def})
	require.NoError(t, err)
	defer srv.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, srv.WaitReady(ctx))

	mgr := &Manager{opts: Options{RoverID: "rover-1", RuntimeDir: runtimeDir, Auth: builder}}
	mgr.SetServer(srv)

	d := &waypointv1.ModuleDesired{
		Id:         "umr",
		Version:    "0.1.0",
		ConfigToml: "[modules_config.umr]\nhost = \"https://192.168.105.1\"\n",
	}
	require.NoError(t, mgr.ensureModuleCreds(ctx, d))

	user, pass := readCredsEnv(t, filepath.Join(runtimeDir, "umr", "creds.env"))
	cfg, err := os.ReadFile(filepath.Join(runtimeDir, "umr", "config.toml"))
	require.NoError(t, err)
	require.Contains(t, string(cfg), "192.168.105.1", "ensureModuleCreds must also restore the module config")

	mc, err := natsgo.Connect(srv.ClientURL(), natsgo.UserInfo(user, pass))
	require.NoError(t, err)
	defer mc.Close()
	errCh := make(chan error, 1)
	mc.SetErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) { errCh <- err })
	require.NoError(t, mc.Publish("waypoint.rover-1.module.umr.uplink", []byte("ok")))
	require.NoError(t, mc.FlushTimeout(time.Second))
	select {
	case err := <-errCh:
		t.Fatalf("unexpected error publishing in own subtree: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	// Idempotent: a second call (e.g. re-attach) must not error or double-add.
	require.NoError(t, mgr.ensureModuleCreds(ctx, d))
}

func readCredsEnv(t *testing.T, path string) (user, pass string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "WAYPOINT_NATS_USER":
			user = v
		case "WAYPOINT_NATS_PASSWORD":
			pass = v
		}
	}
	return
}

func TestActiveComponentsTracksAttachDetach(t *testing.T) {
	pub := NewSnapshotPublisher(nil, "rover-1", nil)
	sup := &fakeSupervisor{active: true}
	m := &Manager{opts: Options{Supervisor: sup, RoverID: "rover-1"}, pub: pub,
		hw: map[string]*HealthWatcher{}, healthPollers: map[string]context.CancelFunc{}}
	man := &Manifest{Component: &Component{Class: "arm"}}
	m.onModuleAttached(context.Background(), "so100", man)
	require.Equal(t, map[string]string{"so100": "arm"}, m.ActiveComponents())
	m.onModuleDetached("so100")
	require.Empty(t, m.ActiveComponents())
}
