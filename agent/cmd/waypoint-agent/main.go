package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/waypointos/waypoint/agent/internal/alerts"
	"github.com/waypointos/waypoint/agent/internal/apply"
	"github.com/waypointos/waypoint/agent/internal/audit"
	"github.com/waypointos/waypoint/agent/internal/basemap"
	"github.com/waypointos/waypoint/agent/internal/cameras"
	"github.com/waypointos/waypoint/agent/internal/cameras/ice"
	"github.com/waypointos/waypoint/agent/internal/cameras/whep"
	"github.com/waypointos/waypoint/agent/internal/clockanchor"
	"github.com/waypointos/waypoint/agent/internal/config"
	"github.com/waypointos/waypoint/agent/internal/diag"
	"github.com/waypointos/waypoint/agent/internal/discovery"
	"github.com/waypointos/waypoint/agent/internal/gateway"
	"github.com/waypointos/waypoint/agent/internal/heartbeat"
	"github.com/waypointos/waypoint/agent/internal/image"
	"github.com/waypointos/waypoint/agent/internal/localauth"
	"github.com/waypointos/waypoint/agent/internal/modules"
	wpnats "github.com/waypointos/waypoint/agent/internal/nats"
	"github.com/waypointos/waypoint/agent/internal/natsunix"
	"github.com/waypointos/waypoint/agent/internal/recorder"
	"github.com/waypointos/waypoint/agent/internal/session"
	"github.com/waypointos/waypoint/agent/internal/sysinfo"
	"github.com/waypointos/waypoint/agent/web"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/protocol/platform/descriptor"
)

func toManagerSpecs(cs []config.CameraSpec) []cameras.CameraSpec {
	out := make([]cameras.CameraSpec, 0, len(cs))
	for _, c := range cs {
		out = append(out, cameras.CameraSpec{Name: c.Name, Device: c.Device, Pipeline: c.Pipeline})
	}
	return out
}

func main() {
	identityPath := flag.String("identity", "/data/waypoint/identity.toml", "path to identity.toml")
	// Default :80 so production is reachable at http://waypoint.local/ with no
	// port. The agent runs as root on the rover and its systemd unit passes no
	// -http flag, so this default applies on deploy. Dev passes -http :8080.
	httpAddr := flag.String("http", ":80", "HTTP listen address")
	flag.Parse()

	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})

	// Until the embedded NATS server is up, slog writes only to stderr. After
	// NATS is ready we re-install with a bus sink as well.
	slog.SetDefault(slog.New(stderrHandler))

	// Bridge stdlib `log` (used by vendor packages) into slog. Each write becomes
	// one slog.Info call. We drop the trailing newline that stdlib log appends.
	log.SetFlags(0)
	log.SetOutput(slogWriter{})

	id, err := config.Load(*identityPath)
	if err != nil {
		slog.Error(fmt.Sprintf("identity: %v", err)); os.Exit(1)
	}
	slog.Info(fmt.Sprintf("waypoint-agent rover=%s name=%q", id.Rover.ID, id.Rover.Name))

	// Logs=true surfaces leaf-node lifecycle (connect/disconnect/auth
	// reject/retry) into journalctl. Without it, dial failures are
	// silent and hard to debug. Tests pass Options{} so output stays
	// quiet there.
	authBuilder := localauth.NewBuilder()

	// Discover and register modules BEFORE the embedded server starts so
	// per-module users + ACLs are baked into the server's static user list.
	// Shared with the gateway so /module/<id>/* reverse-proxy resolves the
	// port the reconciler stamps on attach.
	modReg := gateway.NewModuleProxyRegistry()
	// Dev-image marker: the same WAYPOINT_SKIP_VERIFY=1 env that the verifier
	// already keys on. Dev images set it via /etc/waypoint/agent.env (see
	// image/external/board/raspberrypi5/post-build.sh); prod images leave it
	// unset. Reusing it avoids a second flag to keep in sync.
	isDev := os.Getenv("WAYPOINT_SKIP_VERIFY") == "1"
	modMgr, err := modules.New(modules.Options{
		InstallDir:    envOr("WAYPOINT_MODULE_INSTALL_DIR", "/usr/share/waypoint/modules"),
		ConfigPath:    envOr("WAYPOINT_MODULES_CONFIG", "/data/waypoint/modules.toml"),
		RuntimeDir:    envOr("WAYPOINT_MODULE_RUNTIME_DIR", "/run/waypoint/modules"),
		ConfigDirPath: "/data/waypoint",
		// Bulk .raw payloads and module-private state default to /data/waypoint
		// but on-rover are redirected to the grow-to-fill store partition via
		// env (set in waypoint-agent.service). Sim/tests keep the defaults.
		RawStorageDir: envOr("WAYPOINT_MODULE_RAW_DIR", "/data/waypoint"),
		// The rootfs is a read-only squashfs, so module drop-ins go to tmpfs
		// /run (systemd honours /run/systemd/system/<unit>.d). They are
		// regenerated from the manifest on every boot's reconcile, so losing
		// them on reboot is fine — and it matches portablectl's --runtime attach.
		DropinDir:       "/run/systemd/system",
		ModuleStateRoot: envOr("WAYPOINT_MODULE_STATE_DIR", "/data/waypoint/module-state"),
		IsDevImage:      isDev,
		Auth:          authBuilder,
		Supervisor:    modules.NewDefaultSupervisor(),
		RoverID:       id.Rover.ID,
		ProxyRegistry: modReg,
	})
	if err != nil {
		slog.Warn(fmt.Sprintf("modules: load: %v (continuing without modules)", err))
	}

	users, defaultUser := authBuilder.Build()
	natsOpts := wpnats.Options{Port: 4222, Logs: true, Users: users, NoAuthUser: defaultUser}
	var ns *wpnats.Server
	if id.Proxy.URL != "" {
		slog.Info(fmt.Sprintf("starting nats with leaf node → %s", id.Proxy.URL))
		ns, err = wpnats.StartEmbeddedWithLeaf(natsOpts, id.Proxy.URL, id.NATS.UserJWT, id.NATS.UserSeed)
	} else {
		slog.Info("starting nats in local-only mode (no [proxy] configured)")
		ns, err = wpnats.StartEmbedded(natsOpts)
	}
	if err != nil {
		slog.Error(fmt.Sprintf("nats: %v", err)); os.Exit(1)
	}
	defer ns.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ns.WaitReady(ctx); err != nil {
		slog.Error(fmt.Sprintf("nats ready: %v", err)); os.Exit(1)
	}
	slog.Info(fmt.Sprintf("nats listening at %s", ns.ClientURL()))

	// No-auth connect: mapped to NoAuthUser ("_default") on the embedded
	// server. Module-issued creds are not needed here; broad permissions
	// match historical behavior. See agent/internal/localauth.
	diagNC, err := nats.Connect(ns.ClientURL())
	if err != nil {
		slog.Error(fmt.Sprintf("diag: nats connect: %v", err))
		os.Exit(1)
	}
	busSink := diag.NewBusSink(diagNC, fmt.Sprintf("waypoint.%s.diag.log", id.Rover.ID), slog.LevelInfo)
	slog.SetDefault(slog.New(diag.NewMultiHandler(stderrHandler, busSink)))
	slog.Info("diag: bus sink attached")

	sockPath := os.Getenv("WAYPOINT_NATS_SOCKET")
	if sockPath == "" {
		if runtime.GOOS == "darwin" {
			sockPath = "/tmp/waypoint-nats.sock"
		} else {
			sockPath = "/run/waypoint/nats.sock"
		}
	}
	// Ensure parent dir exists (e.g. /run/waypoint on Linux).
	_ = os.MkdirAll(filepath.Dir(sockPath), 0750)

	relay, err := natsunix.Start(context.Background(), natsunix.Config{
		SocketPath: sockPath,
		BackendURL: ns.ClientURL(),
	})
	if err != nil {
		slog.Error(fmt.Sprintf("natsunix relay: %v", err)); os.Exit(1)
	}
	defer relay.Stop()

	// Shared NATS client connection for image state publishing + apply RPC.
	// No-auth connect: mapped to NoAuthUser ("_default") on the embedded
	// server. Module-issued creds are not needed here; broad permissions
	// match historical behavior. See agent/internal/localauth.
	imgNC, err := nats.Connect(ns.ClientURL())
	if err != nil {
		slog.Error(fmt.Sprintf("image: nats connect: %v", err)); os.Exit(1)
	}
	defer imgNC.Close()

	imgCtx, imgCancel := context.WithCancel(context.Background())
	defer imgCancel()

	if modMgr != nil {
		modMgr.SetServer(ns) // hot-grant NATS creds to runtime-installed modules
		modMgr.SetConn(imgNC)
		go modMgr.Run(imgCtx)
	}

	// Local audit + alerts persistence. Both stores share
	// imgNC for the same reason the alerts publisher does — disjoint subjects,
	// no benefit to a separate connection. Paths default to /data/waypoint and
	// can be overridden for tests / non-standard layouts.
	// TODO(sweep): periodic Sweep goroutine for retention (DefaultMaxAge=30d,
	// DefaultMaxRows=50_000).
	auditPath := os.Getenv("WAYPOINT_AUDIT_DB")
	if auditPath == "" {
		auditPath = "/data/waypoint/audit.db"
	}
	auditStore, err := audit.OpenSQLite(auditPath)
	if err != nil {
		slog.Error(fmt.Sprintf("audit store: %v", err)); os.Exit(1)
	}
	defer auditStore.Close()
	if err := audit.StartLocalSubscriber(imgCtx, imgNC, id.Rover.ID, auditStore); err != nil {
		slog.Error(fmt.Sprintf("audit subscriber: %v", err)); os.Exit(1)
	}

	alertsPath := os.Getenv("WAYPOINT_ALERTS_DB")
	if alertsPath == "" {
		alertsPath = "/data/waypoint/alerts.db"
	}
	alertsStore, err := alerts.OpenSQLite(alertsPath)
	if err != nil {
		slog.Error(fmt.Sprintf("alerts store: %v", err)); os.Exit(1)
	}
	defer alertsStore.Close()
	if err := alerts.StartLocalSubscriber(imgCtx, imgNC, id.Rover.ID, alertsStore); err != nil {
		slog.Error(fmt.Sprintf("alerts subscriber: %v", err)); os.Exit(1)
	}

	imgPub := image.NewPublisher(imgNC, id.Rover.ID, nil)
	if err := imgPub.Start(imgCtx); err != nil {
		slog.Warn(fmt.Sprintf("image publisher: %v (continuing)", err))
	}

	applyH := apply.NewHandler(imgNC, id.Rover.ID)
	// Alerts publisher: reuse imgNC — image apply + alerts
	// publish on disjoint subjects, and a separate connection would just add
	// reconnect surface for no benefit. The hook is intentionally a closure
	// instead of a constructor arg so existing apply tests don't need to
	// know about alerts.
	applyH.OnApplyFailed = func(msg string, md map[string]string) {
		alerts.PublishRaised(imgNC, id.Rover.ID,
			alerts.DeterministicID(id.Rover.ID, "agent.image", "image_apply_failed"),
			waypointv1.Severity_SEVERITY_FAULT,
			"agent.image", "image_apply_failed", msg, md,
		)
	}
	if err := applyH.Start(); err != nil {
		slog.Error(fmt.Sprintf("apply handler: %v", err)); os.Exit(1)
	}

	// System telemetry (this spec, 2026-05-16): 1 Hz heartbeat on
	// waypoint.<rover-id>.telemetry.system. Reuses imgNC + imgCtx because
	// the subject is disjoint from image/audit/alerts and a separate conn
	// would just add reconnect surface. The image closure keeps sysinfo
	// decoupled from agent/internal/image — only main imports both.
	imgLoader := image.DefaultLoader()
	// LeafRTTReader exposes nats-server's leaf-node keepalive RTT, which
	// is what the dashboard's SYSTEM panel and ConnectionPill render as
	// "link RTT". Wire only when a leaf is configured; in local-only
	// mode there's no upstream to measure.
	var rttFn func() (float64, bool)
	if id.Proxy.URL != "" {
		rttReader := sysinfo.NewLeafRTTReader(ns.Server())
		rttFn = rttReader.LatestMs
	}
	sysPub := sysinfo.New(
		imgNC,
		id.Rover.ID,
		sysinfo.DefaultProbes(),
		func() sysinfo.ImageInfo {
			s, err := imgLoader.Load()
			if err != nil || s == nil {
				return sysinfo.ImageInfo{}
			}
			return sysinfo.ImageInfo{Version: s.Version, Partition: s.Partition}
		},
		rttFn,
	)
	go sysPub.Run(imgCtx)

	// Per-boot clock anchor on infra.clock so a recorder can map monotonic
	// capture stamps to wall time. Reuses imgNC/imgCtx like sysinfo.
	go clockanchor.New(imgNC, id.Rover.ID).Run(imgCtx)

	// Heartbeat (agent -> core watchdog, also proxy fleet-liveness):
	// 500ms cadence on waypoint.<rover-id>.infra.heartbeat. Reuses
	// imgNC + imgCtx for the same disjoint-subject reason as sysinfo.
	hbPub := heartbeat.New(imgNC, id.Rover.ID)
	go hbPub.Run(imgCtx)

	// Leaf-watcher: raise proxy_disconnected when the leaf
	// link to the Waypoint proxy hub drops and auto-resolve when it
	// reconnects. Skipped in LAN-only mode where no leaf remote is
	// configured — there's nothing meaningful to watch.
	if id.Proxy.URL != "" {
		proxyAlertID := alerts.DeterministicID(id.Rover.ID, "agent.proxy", "proxy_disconnected")
		watcher := alerts.NewLeafWatcher(ns.Server(), 2*time.Second,
			func() { // onDown
				alerts.PublishRaised(imgNC, id.Rover.ID, proxyAlertID,
					waypointv1.Severity_SEVERITY_FAULT,
					"agent.proxy", "proxy_disconnected",
					"leaf-node connection to proxy lost", nil)
			},
			func() { // onUp
				alerts.PublishResolved(imgNC, id.Rover.ID, proxyAlertID, "leaf reconnected", true)
			},
		)
		leafCtx, leafCancel := context.WithCancel(context.Background())
		defer leafCancel()
		go watcher.Run(leafCtx)
	}

	// Once the agent is up, signal /run/waypoint/healthy so the bootcheck
	// systemd unit clears /data/waypoint/bootcount and the image becomes
	// the steady-state boot target.
	go func() {
		time.Sleep(3 * time.Second)
		if err := image.SignalHealthy(""); err != nil {
			slog.Warn(fmt.Sprintf("image.SignalHealthy: %v", err))
		}
	}()

	// LocalIssuer signs dashboard session JWTs handed out by
	// POST /api/auth/nats-cred. The seed is persisted under /data/waypoint so
	// outstanding session JWTs survive agent restarts; override with
	// WAYPOINT_LOCAL_ACCOUNT_SEED for tests / non-standard layouts.
	seedPath := os.Getenv("WAYPOINT_LOCAL_ACCOUNT_SEED")
	if seedPath == "" {
		seedPath = "/data/waypoint/local-account.nk"
	}
	localIss, err := session.LoadOrCreate(seedPath)
	if err != nil {
		slog.Error(fmt.Sprintf("local issuer: %v", err)); os.Exit(1)
	}

	var modCtl gateway.LocalModules
	if modMgr != nil {
		modCtl = moduleCtl{m: modMgr}
	}
	gw := gateway.New(ns.ClientURL(), id.Rover.ID, id.Local.BootstrapAdminToken, id.Proxy.OperatorPubKey, localIss, modReg, modules.DefaultModuleMountRoot, modCtl)

	gwHandler := gw.HTTPHandler()
	mux := http.NewServeMux()
	mux.Handle("/ws", gwHandler)
	mux.Handle("/login", gwHandler)
	mux.Handle("/api/", gwHandler)
	// Module HTTP surface (/module/<id>/static/* and /module/<id>/* reverse
	// proxy) is registered on the gateway handler in HTTPHandler(); mount it
	// on the outer mux so traffic reaches it.
	mux.Handle("/module/", gwHandler)

	// Cameras: always start the manager. Explicit [[cameras]] in identity.toml
	// are used as-is; with none, the manager auto-discovers capture devices at
	// boot. Either way it publishes the camera list and mounts WHEP routes.
	//
	// No-auth connect: mapped to NoAuthUser ("_default") on the embedded
	// server. Module-issued creds are not needed here; broad permissions
	// match historical behavior. See agent/internal/localauth.
	camNC, err := nats.Connect(ns.ClientURL())
	if err != nil {
		slog.Error(fmt.Sprintf("cameras: nats connect: %v", err)); os.Exit(1)
	}
	defer camNC.Close()

	// Best-effort: ask the proxy for TURN config. LAN-only setups (no
	// proxy URL) skip this — env-provided creds still work via
	// ice.Resolve. Failure is non-fatal; we just fall back to LAN-direct.
	if id.Proxy.URL != "" {
		ice.RequestFromProxy(camNC, id.Rover.ID, 3*time.Second)
	}

	camMgr := cameras.NewManager(cameras.ManagerConfig{
		Cameras: toManagerSpecs(id.Cameras),
		NC:      camNC,
		RoverID: id.Rover.ID,
		IceCfg:  ice.Resolve,
	})
	if err := camMgr.Start(context.Background()); err != nil {
		slog.Error(fmt.Sprintf("cameras: %v", err)); os.Exit(1)
	}
	defer camMgr.Stop()
	whep.New(mux, whep.Resolver{Cameras: camMgr.Camera})
	slog.Info(fmt.Sprintf("cameras: %d camera(s) started, WHEP routes mounted", len(camMgr.List())))

	// Episodic recorder: captures declared telemetry, command
	// streams, active component leaves, and capture-stamped H.264 into per-episode
	// MCAP files. Wired after cameras so the AU tap manager exists.
	episodesDir := envOr("WAYPOINT_EPISODES_DIR", "/data/waypoint/episodes")
	if err := os.MkdirAll(episodesDir, 0o755); err != nil {
		slog.Warn(fmt.Sprintf("recorder: episodes dir: %v", err))
	}
	if err := recorder.RecoverPartials(episodesDir); err != nil {
		slog.Warn(fmt.Sprintf("recorder: recover partials: %v", err))
	}
	var platDesc *descriptor.Descriptor
	platformID := "unknown"
	if p := os.Getenv("WAYPOINT_PLATFORM_DESCRIPTOR"); p != "" {
		if d, err := descriptor.Load(p); err == nil {
			platDesc = d
			platformID = d.Platform.ID
		} else {
			slog.Warn(fmt.Sprintf("recorder: descriptor: %v", err))
		}
	}
	minFreeMB := uint64(500)
	if v := os.Getenv("WAYPOINT_EPISODES_MIN_FREE_MB"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			minFreeMB = n
		}
	}
	rec := recorder.New(recorder.Config{
		NC:         imgNC,
		RoverID:    id.Rover.ID,
		Dir:        episodesDir,
		PlatformID: platformID,
		Specs: func() []recorder.StreamSpec {
			if modMgr == nil {
				return recorder.ResolveSet(platDesc, nil)
			}
			return recorder.ResolveSet(platDesc, modMgr.ActiveComponents())
		},
		Cameras:      camMgr,
		MinFreeBytes: minFreeMB << 20,
		OnAutoStop: func(reason string) {
			alerts.PublishRaised(imgNC, id.Rover.ID,
				alerts.DeterministicID(id.Rover.ID, "agent.recorder", "recording_autostopped"),
				waypointv1.Severity_SEVERITY_WARN,
				"agent.recorder", "recording_autostopped", reason, nil)
		},
	})
	if err := rec.Serve(); err != nil {
		slog.Warn(fmt.Sprintf("recorder: serve: %v", err))
	}
	gw.SetEpisodes(rec)

	bmCache := basemap.NewCache(filepath.Join("/data/waypoint", "basemap"))
	bmFetcher := basemap.NewNATSFetcher(imgNC, id.Rover.ID, 5*time.Second)
	mux.Handle("/basemap/", gw.WithLocalSession(basemap.NewHandler(bmCache, bmFetcher)))

	// Auto-issue the local session cookie on document loads so a LAN visitor
	// is online without pasting ?token=. See gateway.WithLocalSession.
	mux.Handle("/", gw.WithLocalSession(web.Handler()))

	srv := &http.Server{Addr: *httpAddr, Handler: mux}
	go func() {
		slog.Info(fmt.Sprintf("http listening on %s", *httpAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error(fmt.Sprintf("http: %v", err)); os.Exit(1)
		}
	}()

	// Advertise the dashboard over mDNS so it's reachable at
	// http://<name>.local/ on the LAN. Non-fatal: a network that blocks
	// multicast (or Tailscale-only access) just falls back to the IP.
	mdnsHost := id.Local.MDNSHostname
	if mdnsHost == "" {
		mdnsHost = "waypoint"
	}
	if port := discovery.PortFromAddr(*httpAddr); port > 0 {
		if mdnsSrv, err := discovery.Advertise("Waypoint Rover", mdnsHost, port); err != nil {
			slog.Warn(fmt.Sprintf("mdns: advertise %s.local:%d: %v (name-based access disabled)", mdnsHost, port, err))
		} else {
			defer mdnsSrv.Shutdown()
			slog.Info(fmt.Sprintf("mdns: advertising http://%s.local:%d", mdnsHost, port))
		}
	} else {
		slog.Warn(fmt.Sprintf("mdns: cannot parse port from %q, name-based access disabled", *httpAddr))
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	slog.Info("shutting down")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctx2)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// moduleCtl adapts *modules.Manager to gateway.LocalModules without creating an
// import cycle (gateway → modules is forbidden; modules already depends on gateway
// types via the ProxyRegistry interface).
type moduleCtl struct{ m *modules.Manager }

func (c moduleCtl) ListModules() []gateway.ModuleStatusDTO {
	out := make([]gateway.ModuleStatusDTO, 0)
	for _, s := range c.m.ListModules() {
		out = append(out, gateway.ModuleStatusDTO{ID: s.ID, Label: s.Label, Version: s.Version, Origin: s.Origin})
	}
	return out
}

func (c moduleCtl) StageModule(ctx context.Context, raw, bundle []byte) (gateway.StagePreviewDTO, error) {
	p, err := c.m.StageModule(ctx, raw, bundle)
	if err != nil {
		return gateway.StagePreviewDTO{}, err
	}
	return gateway.StagePreviewDTO{
		StageID: p.StageID, ID: p.ID, Version: p.Version, Label: p.Label, SignerSAN: p.SignerSAN,
		UIKind: p.UIKind, TabID: p.TabID, Permissions: p.Permissions, Devices: p.Devices, Pinned: p.Pinned,
	}, nil
}

func (c moduleCtl) ConfirmModule(ctx context.Context, stageID, cfg string) error {
	return c.m.ConfirmModule(ctx, stageID, cfg)
}

func (c moduleCtl) UninstallModule(ctx context.Context, id string) error {
	return c.m.UninstallModule(ctx, id)
}

// slogWriter routes stdlib log.Printf calls into slog. Vendor packages still
// reach for the stdlib log; this bridges them through the structured logger.
type slogWriter struct{}

func (slogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	slog.Info(msg, "via", "stdlib-log")
	return len(p), nil
}
