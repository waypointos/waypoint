package modules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/localauth"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// authReloader hot-swaps the embedded NATS user list without a restart.
// Satisfied by *nats.Server; an interface here keeps the manager testable and
// avoids depending on the concrete server type.
type authReloader interface {
	SetUsers(users []*natsserver.User, noAuthUser string) error
}

// Options configures the Manager. Auth and Supervisor are required at
// construction. NC is set later (after the embedded server starts) via
// SetConn — Manager construction has to precede server start so that
// per-module users land in the auth builder before NewServer is called.
type Options struct {
	InstallDir      string // default /usr/share/waypoint/modules
	ConfigPath      string // default /data/waypoint/modules.toml
	RuntimeDir      string // default /run/waypoint/modules
	ConfigDirPath   string // default "/data/waypoint" — where modules.desired.pb caches
	RawStorageDir   string // default "/data/waypoint"  — root for /data/waypoint/modules/<id>/<ver>.raw
	DropinDir       string // default "/run/systemd/system" — where reconciler writes module drop-ins (tmpfs; the rootfs is read-only)
	ModuleStateRoot string // default "/data/waypoint/module-state" — writable backing for a module's [hardware] read_write paths
	SigstoreRootPath string // default /usr/share/waypoint/sigstore/trusted_root.json
	IsDevImage      bool   // dev images may honour modules.desired.override.toml
	Auth            *localauth.Builder
	Supervisor      Supervisor
	RoverID         string
	// ProxyRegistry is updated by the reconciler whenever a module with a
	// [ui.proxy] manifest attaches or detaches. The gateway shares the same
	// registry so /module/<id>/* reverse-proxy routes resolve immediately.
	ProxyRegistry ProxyRegistry
}

const runtimeHealthPollInterval = 2 * time.Second

// Manager owns discovery, ACL registration, runtime cred writing, supervisor
// invocation, health watching, and snapshot publishing.
type Manager struct {
	opts  Options
	specs []ModuleSpec

	connMu sync.Mutex
	nc     *natsgo.Conn

	// authMu serialises post-boot mutation of opts.Auth + server reload.
	authMu      sync.Mutex
	server      authReloader
	roverConfig map[string]map[string]any // [modules_config.<id>] fallback for installs

	pub           *SnapshotPublisher
	uplinkMir     *uplinkMirror
	servoBkr      *servoBroker
	teleopIn      *teleopInputBroker
	hw            map[string]*HealthWatcher
	pollersMu     sync.Mutex
	healthPollers map[string]context.CancelFunc

	reconcileFn func(ctx context.Context, set *waypointv1.DesiredModuleSet) error

	desiredMu    sync.Mutex
	proxyDesired *waypointv1.DesiredModuleSet
	localDesired *waypointv1.DesiredModuleSet
	runCtx       context.Context // set in Run; used by HTTP-driven installs

	portable   PortableImage
	verifier   signatureVerifier
	verifyOnce sync.Once
	verifyErr  error
	stagingMu  sync.Mutex
	staging    map[string]stagedModule

	componentsMu sync.Mutex
	components   map[string][]string // module id -> component classes
}

// New discovers modules and adds their users to the localauth Builder.
// MUST be called BEFORE the embedded NATS server is started, because
// localauth.Builder.Build() snapshots its users.
func New(opts Options) (*Manager, error) {
	specs, err := Discover(opts.InstallDir, opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("modules: discover: %w", err)
	}
	// Provision installed (desired/local) module creds FIRST so an installed
	// module's permissions win over a stale baked spec of the same id. A module
	// can be both baked into an older image and installed at runtime; the runtime
	// version is what the reconciler actually attaches, so its creds (the full
	// module.<id>.> subtree) must win — otherwise a baked manifest's narrower,
	// outdated publish list shadows the install and the module's NATS publishes
	// are denied.
	cachePath := filepath.Join(opts.ConfigDirPath, "modules.desired.pb")
	localPath := filepath.Join(opts.ConfigDirPath, "modules.local.pb")
	// Config fallback for installed modules whose desired entry has no ConfigToml.
	// Read straight from the [modules_config.<id>] tables, independent of any
	// [[modules]] enable entry, so an install-only module keeps its settings.
	var roverConfig map[string]map[string]any
	if data, err := os.ReadFile(opts.ConfigPath); err == nil {
		roverConfig, _ = decodeModulesConfig(string(data))
	}
	provisioned := map[string]bool{}
	for _, id := range provisionCachedDesiredCreds(cachePath, opts.RawStorageDir, opts.RuntimeDir, opts.RoverID, opts.Auth, roverConfig) {
		provisioned[id] = true
	}
	for _, id := range provisionCachedDesiredCreds(localPath, opts.RawStorageDir, opts.RuntimeDir, opts.RoverID, opts.Auth, roverConfig) {
		provisioned[id] = true
	}
	for i := range specs {
		if provisioned[specs[i].Manifest.Name] {
			continue // an installed copy already claimed this module's creds
		}
		creds, err := opts.Auth.AddModuleUser(specs[i].Manifest.Name, opts.RoverID, ManifestAuthPermissions(&specs[i].Manifest))
		if err != nil {
			return nil, fmt.Errorf("modules: %s: %w", specs[i].Manifest.Name, err)
		}
		if _, err := WriteRuntimeFiles(opts.RuntimeDir, specs[i], creds); err != nil {
			return nil, fmt.Errorf("modules: %s: %w", specs[i].Manifest.Name, err)
		}
	}
	return &Manager{opts: opts, specs: specs, roverConfig: roverConfig, hw: map[string]*HealthWatcher{}, healthPollers: map[string]context.CancelFunc{}}, nil
}

// SetConn provides the agent's nats connection. Must be called before Run.
func (m *Manager) SetConn(nc *natsgo.Conn) {
	m.connMu.Lock()
	defer m.connMu.Unlock()
	m.nc = nc
}

// SetServer provides the embedded NATS server so a module installed at runtime
// can be granted creds live (see ensureModuleCreds). Must be called before Run.
func (m *Manager) SetServer(s authReloader) {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	m.server = s
}

// ensureModuleCreds grants a module its NATS user just before it starts, so an
// install applies without a reboot. No-op if the module already has a user (boot
// or earlier this session); otherwise mints the full module.<id>.> subtree,
// writes creds.env + config.toml, and hot-reloads the server's auth.
func (m *Manager) ensureModuleCreds(_ context.Context, d *waypointv1.ModuleDesired) error {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	if m.opts.Auth == nil || m.opts.Auth.HasUser(d.Id) {
		return nil
	}
	perms := localauth.ModulePermissions{
		Publish:   []string{"waypoint.*.module." + d.Id + ".>"},
		Subscribe: []string{"waypoint.*.module." + d.Id + ".>"},
	}
	creds, err := m.opts.Auth.AddModuleUser(d.Id, m.opts.RoverID, perms)
	if err != nil {
		return fmt.Errorf("modules: ensure creds %s: %w", d.Id, err)
	}
	spec := ModuleSpec{Manifest: Manifest{Name: d.Id}, Config: desiredModuleConfig(d, m.roverConfig)}
	if _, err := WriteRuntimeFiles(m.opts.RuntimeDir, spec, creds); err != nil {
		return fmt.Errorf("modules: ensure creds %s: %w", d.Id, err)
	}
	if m.server != nil {
		users, defaultUser := m.opts.Auth.Build()
		if err := m.server.SetUsers(users, defaultUser); err != nil {
			return fmt.Errorf("modules: reload creds %s: %w", d.Id, err)
		}
	}
	return nil
}

func (m *Manager) proxyCachePath() string { return filepath.Join(m.opts.ConfigDirPath, "modules.desired.pb") }
func (m *Manager) localStorePath() string { return filepath.Join(m.opts.ConfigDirPath, "modules.local.pb") }
func (m *Manager) trustPath() string      { return filepath.Join(m.opts.ConfigDirPath, "modules.trust.toml") }

// reconcileMerged reconciles the union of the proxy and local desired sets
// (local wins on id). Acquires desiredMu internally; callers must not hold it.
func (m *Manager) reconcileMerged(ctx context.Context) error {
	m.desiredMu.Lock()
	merged := mergeDesired(m.proxyDesired, m.localDesired)
	m.desiredMu.Unlock()
	if m.reconcileFn == nil {
		return nil
	}
	return m.reconcileFn(ctx, merged)
}

// isLocalID reports whether id is in the local desired set (→ origin LOCAL).
func (m *Manager) isLocalID(id string) bool {
	m.desiredMu.Lock()
	defer m.desiredMu.Unlock()
	return containsModule(m.localDesired, id)
}

// Run starts the supervisor, health watchers, and snapshot publisher.
// Blocks until ctx fires. Safe to call when no modules are enabled
// (returns after publishing one empty snapshot).
func (m *Manager) Run(ctx context.Context) {
	m.connMu.Lock()
	nc := m.nc
	m.connMu.Unlock()
	if nc == nil {
		slog.Warn("modules: Run called without SetConn; nothing to do")
		return
	}
	m.pub = NewSnapshotPublisher(nc, m.opts.RoverID, m.specs)
	m.uplinkMir = newUplinkMirror(nc, m.opts.RoverID)
	m.servoBkr = newServoBroker(nc, m.opts.RoverID, loadDenyIDs([]string{
		os.Getenv("WAYPOINT_PLATFORM_DESCRIPTOR"),
		"/data/waypoint/platform.toml",
		"/usr/share/waypoint/platform.toml",
	}))
	m.teleopIn = newTeleopInputBroker(nc, m.opts.RoverID)

	// Dev images launch modules out-of-band (make dev-module, the sim
	// conformance harness) so no per-module attach fires. Start a wildcard servo
	// broker so servo-control reaches those modules; the deny-list still applies.
	if m.opts.IsDevImage {
		if err := m.servoBkr.startDev(ctx); err != nil {
			slog.Warn(fmt.Sprintf("modules: dev servo broker: %v", err))
		}
	}

	// Reconciler-attached modules ship via portablectl post-boot; EnsureCreds
	// grants their NATS auth live (before start), so an install applies without
	// a reboot. Modules already provisioned at boot are a no-op there.
	m.portable = NewPortable()
	reconciler := &Reconciler{
		RootDir:         m.opts.RawStorageDir,
		Portable:        m.portable,
		Fetcher:         NewFetcher(nil),
		Supervisor:      m.opts.Supervisor,
		ProxyRegistry:    m.opts.ProxyRegistry,
		DropinDir:        m.opts.DropinDir,
		StateBackingRoot: m.opts.ModuleStateRoot,
		ModuleMountRoot:  DefaultModuleMountRoot,
		ReloadUnits:      reloadSystemdUnits,
	}
	m.reconcileFn = reconciler.Reconcile
	reconciler.EnsureCreds = m.ensureModuleCreds
	reconciler.SyncConfig = m.writeDesiredConfig
	reconciler.OnAttach = func(id string, mf *Manifest) { m.onModuleAttached(ctx, id, mf) }
	reconciler.OnDetach = func(id string) { m.onModuleDetached(id) }

	_ = os.RemoveAll(filepath.Join(m.opts.RawStorageDir, "modules", ".staging"))
	m.runCtx = ctx
	cached, cachedErr := ReadDesiredCache(m.proxyCachePath())
	local, localErr := ReadLocalDesired(m.localStorePath())
	m.desiredMu.Lock()
	if cachedErr == nil {
		m.proxyDesired = cached
	}
	if localErr == nil {
		m.localDesired = local
	}
	m.desiredMu.Unlock()
	if err := m.loadDevOverrideIntoLocal(); err != nil {
		slog.Warn(fmt.Sprintf("modules: dev override: %v", err))
	}
	_ = m.reconcileMerged(ctx)

	sub, err := nc.Subscribe(fmt.Sprintf("waypoint.%s.modules.desired", m.opts.RoverID), func(msg *natsgo.Msg) {
		m.handleDesired(ctx, msg)
	})
	if err != nil {
		slog.Warn(fmt.Sprintf("modules: subscribe desired: %v", err))
	} else {
		defer sub.Unsubscribe()
	}

	for _, s := range m.specs {
		if err := m.opts.Supervisor.Start(ctx, s.Manifest.Name); err != nil {
			slog.Warn(fmt.Sprintf("modules: start %s: %v", s.Manifest.Name, err))
			continue
		}
		probe := s.Manifest.Health.Probe
		if probe == "" {
			probe = "ready"
		}
		interval := s.Manifest.Health.Interval
		if interval == 0 {
			interval = 5 * time.Second
		}
		timeout := s.Manifest.Health.Timeout
		if timeout == 0 {
			timeout = 2 * time.Second
		}
		hw := NewHealthWatcher(nc, m.opts.RoverID, s.Manifest.Name, probe, interval, timeout)
		modID := s.Manifest.Name
		hw.OnChange = func(healthy bool) { m.pub.SetHealth(modID, healthy) }
		m.hw[modID] = hw
		go hw.Run(ctx)
	}
	m.pub.Run(ctx)
}

// onModuleAttached registers a runtime module in the snapshot and starts an
// IsActive health poll. PROXY and STATIC alike report health by unit liveness.
func (m *Manager) onModuleAttached(ctx context.Context, id string, manifest *Manifest) {
	if manifest == nil {
		slog.Warn(fmt.Sprintf("modules: attached %s has no readable manifest; not surfacing a tab", id))
		return
	}
	if len(manifest.Components) > 0 {
		classes := make([]string, 0, len(manifest.Components))
		for _, c := range manifest.Components {
			classes = append(classes, c.Class)
		}
		m.componentsMu.Lock()
		if m.components == nil {
			m.components = map[string][]string{}
		}
		m.components[id] = classes
		m.componentsMu.Unlock()
	}
	if containsString(manifest.Provides, "uplink") {
		if err := m.uplinkMir.start(m.runCtxOr(ctx), id); err != nil {
			slog.Warn(fmt.Sprintf("modules: uplink mirror %s: %v", id, err))
		}
	}
	// Dev images already run the wildcard servo broker (startDev); a second
	// per-module relay would double-publish commands and race read replies.
	if containsString(manifest.Requires, "servo-control") && !m.opts.IsDevImage {
		if err := m.servoBkr.start(m.runCtxOr(ctx), id); err != nil {
			slog.Warn(fmt.Sprintf("modules: servo broker %s: %v", id, err))
		}
	}
	if containsString(manifest.Requires, "teleop-input") {
		if err := m.teleopIn.start(m.runCtxOr(ctx), id); err != nil {
			slog.Warn(fmt.Sprintf("modules: teleop-input broker %s: %v", id, err))
		}
	}
	// A teleop-only module has no tab kind but still publishes a window descriptor.
	if manifest.UI.Kind == UIKindNone && manifest.UI.Teleop == nil {
		return // no UI → no tab → nothing to publish
	}
	origin := waypointv1.ModuleOrigin_MODULE_ORIGIN_PROXY
	if m.isLocalID(id) {
		origin = waypointv1.ModuleOrigin_MODULE_ORIGIN_LOCAL
	}
	m.pub.SetRuntimeModule(id, *manifest, origin)
	pctx, cancel := context.WithCancel(ctx)
	m.pollersMu.Lock()
	if m.healthPollers == nil {
		m.healthPollers = map[string]context.CancelFunc{}
	}
	if old := m.healthPollers[id]; old != nil {
		old()
	}
	m.healthPollers[id] = cancel
	m.pollersMu.Unlock()
	go m.pollHealth(pctx, id)
}

func (m *Manager) onModuleDetached(id string) {
	m.pollersMu.Lock()
	if cancel := m.healthPollers[id]; cancel != nil {
		cancel()
		delete(m.healthPollers, id)
	}
	m.pollersMu.Unlock()
	m.componentsMu.Lock()
	delete(m.components, id)
	m.componentsMu.Unlock()
	m.uplinkMir.stop(id)
	m.servoBkr.stop(id)
	m.teleopIn.stop(id)
	m.pub.RemoveRuntimeModule(id)
}

// ActiveComponents reports the component classes of every attached module,
// keyed by module id, manifest order. The recorder snapshots this at
// episode start.
func (m *Manager) ActiveComponents() map[string][]string {
	m.componentsMu.Lock()
	defer m.componentsMu.Unlock()
	out := make(map[string][]string, len(m.components))
	for k, v := range m.components {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func (m *Manager) pollHealth(ctx context.Context, id string) {
	t := time.NewTicker(runtimeHealthPollInterval)
	defer t.Stop()
	check := func() {
		active, err := m.opts.Supervisor.IsActive(ctx, id)
		m.pub.SetHealth(id, err == nil && active)
	}
	check() // immediate first sample
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

// handleDesired is the NATS subscription callback for
// waypoint.<rover-id>.modules.desired. Updates the proxy store and triggers
// a merged reconcile so local modules are not detached.
func (m *Manager) handleDesired(ctx context.Context, msg *natsgo.Msg) {
	var set waypointv1.DesiredModuleSet
	if err := proto.Unmarshal(msg.Data, &set); err != nil {
		slog.Warn(fmt.Sprintf("modules: bad desired payload: %v", err))
		return
	}
	if err := WriteDesiredCache(m.proxyCachePath(), &set); err != nil {
		slog.Warn(fmt.Sprintf("modules: cache desired: %v", err))
	}
	m.desiredMu.Lock()
	m.proxyDesired = &set
	m.desiredMu.Unlock()
	if err := m.reconcileMerged(ctx); err != nil {
		slog.Warn(fmt.Sprintf("modules: reconcile: %v", err))
	}
}

// loadDevOverrideIntoLocal folds dev override entries into the local desired
// store so the single merged reconcile sees them alongside proxy modules.
func (m *Manager) loadDevOverrideIntoLocal() error {
	if !m.opts.IsDevImage {
		return nil
	}
	path := filepath.Join(m.opts.ConfigDirPath, "modules.desired.override.toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var raw overrideFile
	if _, err := toml.NewDecoder(strings.NewReader(string(data))).Decode(&raw); err != nil {
		return fmt.Errorf("override: decode: %w", err)
	}
	m.desiredMu.Lock()
	defer m.desiredMu.Unlock()
	if m.localDesired == nil {
		m.localDesired = &waypointv1.DesiredModuleSet{}
	}
	for _, mod := range raw.Modules {
		m.localDesired = upsertModule(m.localDesired, &waypointv1.ModuleDesired{
			Id: mod.ID, Version: mod.Version, RawUrl: "file://" + mod.RawPath, ConfigToml: mod.Config,
		})
	}
	return nil
}

// provisionCachedDesiredCreds mints a NATS user + writes runtime creds for each
// module in the cached desired set whose .raw is already on disk. Runs at boot
// (before the embedded NATS server starts) so reconciler-attached modules that
// speak NATS have credentials after a reboot. Returns the ids provisioned, so
// the caller can skip a baked spec of the same id (installed copy wins).
func provisionCachedDesiredCreds(cachePath, rawRoot, runtimeDir, roverID string, b *localauth.Builder, roverConfig map[string]map[string]any) []string {
	set, err := ReadDesiredCache(cachePath)
	if err != nil || set == nil {
		return nil
	}
	var ids []string
	for _, d := range set.Modules {
		rawPath := filepath.Join(rawRoot, "modules", d.Id, d.Version, rawImageName(d.Id))
		if _, err := os.Stat(rawPath); err != nil {
			continue // .raw not fetched yet; skip until present
		}
		perms := localauth.ModulePermissions{
			Publish:   []string{"waypoint.*.module." + d.Id + ".>"},
			Subscribe: []string{"waypoint.*.module." + d.Id + ".>"},
		}
		creds, err := b.AddModuleUser(d.Id, roverID, perms)
		if err != nil {
			continue
		}
		spec := ModuleSpec{Manifest: Manifest{Name: d.Id}, Config: desiredModuleConfig(d, roverConfig)}
		_, _ = WriteRuntimeFiles(runtimeDir, spec, creds)
		ids = append(ids, d.Id)
	}
	return ids
}

// writeDesiredConfig writes only config.toml, so a config change reaches a
// module that already holds its NATS user (ensureModuleCreds is a no-op then).
func (m *Manager) writeDesiredConfig(_ context.Context, d *waypointv1.ModuleDesired) error {
	dir := filepath.Join(m.opts.RuntimeDir, d.Id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("modules: config dir %s: %w", d.Id, err)
	}
	cfg := desiredModuleConfig(d, m.roverConfig)
	if cfg == nil {
		cfg = map[string]any{}
	}
	if err := writeTOML(filepath.Join(dir, "config.toml"), cfg); err != nil {
		return fmt.Errorf("modules: write config %s: %w", d.Id, err)
	}
	return nil
}

// desiredModuleConfig resolves a module's config for the boot-time config.toml
// (/run is tmpfs, so this is what restores settings after a reboot). The
// install-time ConfigToml wins; otherwise fall back to modules.toml.
func desiredModuleConfig(d *waypointv1.ModuleDesired, roverConfig map[string]map[string]any) map[string]any {
	if d.GetConfigToml() != "" {
		if cfgs, err := decodeModulesConfig(d.GetConfigToml()); err == nil {
			if cfg := cfgs[d.Id]; cfg != nil {
				return cfg
			}
		}
	}
	return roverConfig[d.Id]
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// overrideFile is the dev-image side channel: lets a developer point the
// reconciler at a local .raw without going through the proxy. Production
// images never read this file (gated by Options.IsDevImage).
type overrideFile struct {
	Modules []struct {
		ID      string `toml:"id"`
		Version string `toml:"version"`
		RawPath string `toml:"raw_path"`
		Config  string `toml:"config"`
	} `toml:"modules"`
}
