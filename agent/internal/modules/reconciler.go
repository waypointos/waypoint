package modules

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// FetcherI lets the reconciler use either the real Fetcher or a test fake.
type FetcherI interface {
	Fetch(ctx context.Context, url, sha256Hex, dest string) error
}

// SupervisorIface is the subset of Supervisor the reconciler needs.
type SupervisorIface interface {
	Start(ctx context.Context, moduleID string) error
	Stop(ctx context.Context, moduleID string) error
}

// ProxyRegistry is satisfied by *gateway.ModuleProxyRegistry but typed here
// as an interface to avoid an import cycle (modules and gateway are siblings).
type ProxyRegistry interface {
	Set(moduleID string, port uint16)
	Clear(moduleID string)
}

// Reconciler diffs the desired module set against the attached set and
// drives portablectl + the supervisor to bring them into alignment.
type Reconciler struct {
	RootDir       string // /data/waypoint/modules/
	Portable      PortableImage
	Fetcher       FetcherI
	Supervisor    SupervisorIface
	ProxyRegistry ProxyRegistry
	DropinDir     string // typically "/run/systemd/system" (tmpfs; rootfs is read-only); "" disables drop-in writes
	// StateBackingRoot is the writable /data root a module's [hardware]
	// read_write paths are bound from: the declared /var/lib/... path sits
	// inside the read-only image and is bound from a read-only host path, so
	// writes need a /data-backed source. "" keeps the legacy host-path bind
	// (tests, writable-rootfs hosts).
	StateBackingRoot string
	// ModuleMountRoot is where each attached image is mounted read-only so the
	// manifest and dashboard assets are readable on the host; "" disables (tests).
	ModuleMountRoot string
	// ReloadUnits runs `systemctl daemon-reload` after drop-ins are written and
	// before a module starts, so freshly-written drop-ins are picked up (the
	// portablectl attach reload ran before they existed). nil disables (tests).
	ReloadUnits func(ctx context.Context) error
	OnAttach        func(id string, m *Manifest) // fired after a successful attach
	OnDetach        func(id string)              // fired after a successful detach
	// EnsureCreds runs just before a module is started, so a runtime install can
	// be granted its NATS creds live (the module reads creds.env on startup). nil
	// disables (tests, boot-provisioned modules already have creds).
	EnsureCreds func(ctx context.Context, d *waypointv1.ModuleDesired) error

	current map[string]string // moduleID → currently-attached raw path
}

// DefaultModuleMountRoot is where the agent mounts attached module images
// read-only. The gateway serves /module/<id>/static/* from
// <DefaultModuleMountRoot>/<id>/dashboard/.
const DefaultModuleMountRoot = "/run/waypoint/module-root"

// rawImageName is the on-disk image filename. portablectl derives the portable
// name from this stem, so it must be "<id>.raw" for `detach <id>` and the
// <id>-keyed mount paths to line up. The version lives in the parent directory.
func rawImageName(id string) string { return id + ".raw" }

// rawDestPath is a module image's storage path: <root>/modules/<id>/<version>/<id>.raw.
func rawDestPath(rootDir, id, version string) string {
	return filepath.Join(rootDir, "modules", id, version, rawImageName(id))
}

func (r *Reconciler) rawDest(id, version string) string {
	return rawDestPath(r.RootDir, id, version)
}

// moduleMount is where the agent mounts module <id>'s image; "" when disabled.
func (r *Reconciler) moduleMount(id string) string {
	if r.ModuleMountRoot == "" {
		return ""
	}
	return filepath.Join(r.ModuleMountRoot, id)
}

func (r *Reconciler) Reconcile(ctx context.Context, desired *waypointv1.DesiredModuleSet) error {
	if r.current == nil {
		r.current = map[string]string{}
	}
	want := map[string]*waypointv1.ModuleDesired{}
	for _, m := range desired.Modules {
		want[m.Id] = m
	}

	for id, rawPath := range r.current {
		w, present := want[id]
		if !present {
			if err := r.detach(ctx, id); err != nil {
				return err
			}
			continue
		}
		if rawPath != r.rawDest(id, w.Version) { // desired version changed
			if err := r.detach(ctx, id); err != nil {
				return err
			}
		}
	}

	for id, m := range want {
		dest := r.rawDest(id, m.Version)
		if r.current[id] == dest {
			continue // already attached at the desired version
		}
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			if err := r.Fetcher.Fetch(ctx, m.RawUrl, m.RawSha256, dest); err != nil {
				slog.Warn(fmt.Sprintf("reconcile: fetch %s@%s: %v", id, m.Version, err))
				continue
			}
		}
		if err := r.attach(ctx, id, dest); err != nil {
			slog.Warn(fmt.Sprintf("reconcile: attach %s: %v", id, err))
			continue
		}
		// Manifest read is non-fatal (no mount root in tests); drop-ins + proxy
		// registry only update when the manifest is readable.
		manifest := r.readManifest(ctx, id, dest)
		if manifest != nil {
			if dropinErr := r.writeHardwareDropin(id, manifest.Hardware); dropinErr != nil {
				slog.Warn(fmt.Sprintf("reconcile: dropin %s: %v", id, dropinErr))
			}
			if dropinErr := r.writeComponentDropin(id, manifest.Component); dropinErr != nil {
				slog.Warn(fmt.Sprintf("reconcile: component dropin %s: %v", id, dropinErr))
			}
			if r.ProxyRegistry != nil && manifest.UI.Kind == UIKindProxy {
				r.ProxyRegistry.Set(id, manifest.UI.ProxyPort)
			}
			// Pick up the drop-ins just written: the attach above reloaded the
			// daemon before they existed, and `systemctl start` alone won't
			// re-read them. Non-fatal — without it the module starts stale.
			if r.ReloadUnits != nil && r.DropinDir != "" {
				if err := r.ReloadUnits(ctx); err != nil {
					slog.Warn(fmt.Sprintf("reconcile: daemon-reload %s: %v", id, err))
				}
			}
		}
		if r.EnsureCreds != nil {
			if err := r.EnsureCreds(ctx, m); err != nil {
				slog.Warn(fmt.Sprintf("reconcile: ensure creds %s: %v", id, err))
				continue
			}
		}
		if err := r.Supervisor.Start(ctx, id); err != nil {
			slog.Warn(fmt.Sprintf("reconcile: start %s: %v", id, err))
			continue
		}
		r.current[id] = dest
		if r.OnAttach != nil {
			r.OnAttach(id, manifest)
		}
	}
	return nil
}

// attach attaches the image, retrying once behind a detach. A --runtime attach
// lives in /run and outlives both a failed start and the agent process, while
// the map that records it does not. portablectl refuses to attach over an
// existing attachment, so without the retry an image left attached by an
// earlier pass wedges the module out of the snapshot until the next reboot.
func (r *Reconciler) attach(ctx context.Context, id, dest string) error {
	err := r.Portable.Attach(ctx, dest)
	if err == nil {
		return nil
	}
	// Stop is best-effort: portablectl will not detach a running unit, and a
	// unit that is already inactive (or absent) makes this a no-op.
	_ = r.Supervisor.Stop(ctx, id)
	if detachErr := r.Portable.Detach(ctx, id); detachErr != nil {
		return fmt.Errorf("%w (detach for retry: %v)", err, detachErr)
	}
	return r.Portable.Attach(ctx, dest)
}

// readManifest mounts module <id>'s attached image read-only and parses its
// module.toml. Returns nil (non-fatal) when no mount root is configured or the
// image is unreadable; the module then attaches without surfacing a tab.
func (r *Reconciler) readManifest(ctx context.Context, id, rawPath string) *Manifest {
	mountPath := r.moduleMount(id)
	if mountPath == "" {
		return nil
	}
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		slog.Warn(fmt.Sprintf("reconcile: mkdir mount %s: %v", id, err))
		return nil
	}
	if err := r.Portable.Mount(ctx, rawPath, mountPath); err != nil {
		slog.Warn(fmt.Sprintf("reconcile: mount %s: %v", id, err))
		return nil
	}
	mf, err := readAttachedManifest(mountPath, id)
	if err != nil {
		slog.Warn(fmt.Sprintf("reconcile: manifest %s: %v", id, err))
		return nil
	}
	return mf
}

func (r *Reconciler) detach(ctx context.Context, id string) error {
	if err := r.Supervisor.Stop(ctx, id); err != nil {
		return fmt.Errorf("reconcile: stop %s: %w", id, err)
	}
	if mountPath := r.moduleMount(id); mountPath != "" {
		if err := r.Portable.Unmount(ctx, mountPath); err != nil {
			slog.Warn(fmt.Sprintf("reconcile: unmount %s: %v", id, err))
		}
		_ = os.Remove(mountPath)
	}
	if err := r.Portable.Detach(ctx, id); err != nil {
		return fmt.Errorf("reconcile: detach %s: %w", id, err)
	}
	if r.ProxyRegistry != nil {
		r.ProxyRegistry.Clear(id)
	}
	delete(r.current, id)
	if r.OnDetach != nil {
		r.OnDetach(id)
	}
	return nil
}

// readAttachedManifest parses module.toml from the read-only image mount at
// mountPath/usr/share/waypoint/modules/<id>/module.toml.
func readAttachedManifest(mountPath, id string) (*Manifest, error) {
	path := filepath.Join(mountPath, "usr/share/waypoint/modules", id, "module.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

// writeHardwareDropin writes the rendered [hardware] systemd drop-in next
// to the module service unit. A no-op when DropinDir is unset (tests) or
// the module has no hardware declarations.
func (r *Reconciler) writeHardwareDropin(moduleID string, h Hardware) error {
	if r.DropinDir == "" {
		return nil
	}
	// Back each read_write path with a persistent, writable dir on /data. The
	// declared path (e.g. /var/lib/waypoint-module-<id>) lives inside the
	// read-only image and is bound from a read-only host path, so the module
	// can only write if BindPaths uses a /data-backed source. The source must
	// exist before the unit starts, so mkdir it here.
	var rwSource func(string) string
	if r.StateBackingRoot != "" {
		rwSource = func(p string) string { return filepath.Join(r.StateBackingRoot, p) }
		for _, p := range h.ReadWrite {
			if err := os.MkdirAll(rwSource(p), 0o755); err != nil {
				return err
			}
		}
	}
	body := RenderHardwareDropin(h, rwSource)
	if body == "" {
		return nil
	}
	dir := filepath.Join(r.DropinDir, "waypoint-module-"+moduleID+".service.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "30-hardware.conf"), []byte(body), 0o644)
}

// writeComponentDropin writes the [component] env drop-in next to the module
// service unit. A no-op when DropinDir is unset (tests) or the module declares
// no component.
func (r *Reconciler) writeComponentDropin(moduleID string, c *Component) error {
	if r.DropinDir == "" {
		return nil
	}
	body := RenderComponentDropin(c)
	if body == "" {
		return nil
	}
	dir := filepath.Join(r.DropinDir, "waypoint-module-"+moduleID+".service.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "40-component.conf"), []byte(body), 0o644)
}
