package modules

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/waypointos/waypoint/modverify"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// signatureVerifier is the subset of modverify the installer needs; an interface
// so tests can inject a fake (real verification needs a baked Sigstore root).
type signatureVerifier interface {
	Verify(ctx context.Context, raw, bundleJSON []byte, sanPattern string) (string, error)
}

// StagePreview is returned by StageModule to let the caller inspect what will
// be installed before committing.
type StagePreview struct {
	StageID     string   `json:"stageId"`
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Label       string   `json:"label"`
	SignerSAN   string   `json:"signerSan"`
	UIKind      string   `json:"uiKind"`
	TabID       string   `json:"tabId"`
	Permissions []string `json:"permissions"`
	Devices     []string `json:"devices"`
	Pinned      bool     `json:"pinned"`
}

// ModuleStatus is returned by ListModules.
type ModuleStatus struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Version string `json:"version"`
	Origin  string `json:"origin"`
}

type stagedModule struct {
	rawPath   string
	stageDir  string
	id        string
	version   string
	signerSAN string
}

// getVerifier returns the injected verifier (tests) or lazily builds one from
// the baked trusted_root.json (production).
func (m *Manager) getVerifier() (signatureVerifier, error) {
	// Fast path: test-injected verifier or already built by verifyOnce below.
	if m.verifier != nil {
		return m.verifier, nil
	}
	m.verifyOnce.Do(func() {
		path := m.opts.SigstoreRootPath
		if path == "" {
			path = "/usr/share/waypoint/sigstore/trusted_root.json"
		}
		v, err := modverify.NewFromFile(path)
		if err != nil {
			m.verifyErr = err
			return
		}
		m.verifier = v
	})
	return m.verifier, m.verifyErr
}

func (m *Manager) getPortable() PortableImage {
	if m.portable != nil {
		return m.portable
	}
	return NewPortable()
}

func (m *Manager) runCtxOr(ctx context.Context) context.Context {
	if m.runCtx != nil {
		return m.runCtx
	}
	return ctx
}

// StageModule verifies the cosign bundle, writes the .raw to a staging
// directory, mounts it to read the manifest, and returns a preview for
// confirmation. The caller must call ConfirmModule or discard.
func (m *Manager) StageModule(ctx context.Context, raw, bundle []byte) (StagePreview, error) {
	v, err := m.getVerifier()
	if err != nil {
		return StagePreview{}, fmt.Errorf("verifier: %w", err)
	}
	san, err := v.Verify(ctx, raw, bundle, modverify.AnyReleaseSAN)
	if err != nil {
		return StagePreview{}, fmt.Errorf("signature: %w", err)
	}

	stageID, err := newStageID()
	if err != nil {
		return StagePreview{}, fmt.Errorf("stage id: %w", err)
	}
	stageDir := filepath.Join(m.opts.RawStorageDir, "modules", ".staging", stageID)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return StagePreview{}, fmt.Errorf("stage dir: %w", err)
	}
	rawPath := filepath.Join(stageDir, "image.raw")
	if err := os.WriteFile(rawPath, raw, 0o644); err != nil {
		_ = os.RemoveAll(stageDir)
		return StagePreview{}, fmt.Errorf("stage write: %w", err)
	}

	id, mf, err := m.readUploadedManifest(ctx, stageID, rawPath)
	if err != nil {
		_ = os.RemoveAll(stageDir)
		return StagePreview{}, fmt.Errorf("manifest: %w", err)
	}

	m.stagingMu.Lock()
	if m.staging == nil {
		m.staging = map[string]stagedModule{}
	}
	m.staging[stageID] = stagedModule{
		rawPath:   rawPath,
		stageDir:  stageDir,
		id:        id,
		version:   mf.Version,
		signerSAN: san,
	}
	m.stagingMu.Unlock()

	return StagePreview{
		StageID:     stageID,
		ID:          id,
		Version:     mf.Version,
		Label:       mf.Label,
		SignerSAN:   san,
		UIKind:      uiKindString(mf.UI.Kind),
		TabID:       mf.UI.TabID,
		Permissions: flattenPerms(mf.Permissions),
		// append to a non-nil slice so JSON encodes [] not null (a module with no
		// declared devices would otherwise crash array-length checks in the UI).
		Devices: append([]string{}, mf.Hardware.Devices...),
		Pinned:      hasPin(m.trustPath(), id),
	}, nil
}

// readUploadedManifest mounts the staged .raw, finds the single module.toml,
// parses it, and returns (name, manifest, err). The mount is torn down before
// returning.
func (m *Manager) readUploadedManifest(ctx context.Context, stageID, rawPath string) (string, *Manifest, error) {
	mountPath := filepath.Join(m.opts.RuntimeDir, ".stage-mnt", stageID)
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		return "", nil, fmt.Errorf("stage mount dir: %w", err)
	}
	defer func() {
		_ = m.getPortable().Unmount(ctx, mountPath)
		_ = os.Remove(mountPath)
	}()

	if err := m.getPortable().Mount(ctx, rawPath, mountPath); err != nil {
		return "", nil, fmt.Errorf("mount: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(mountPath, "usr/share/waypoint/modules/*/module.toml"))
	if err != nil {
		return "", nil, fmt.Errorf("glob manifest: %w", err)
	}
	if len(matches) != 1 {
		return "", nil, fmt.Errorf("expected exactly one module.toml, found %d", len(matches))
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		return "", nil, fmt.Errorf("read manifest: %w", err)
	}
	mf, err := ParseManifest(data)
	if err != nil {
		return "", nil, err
	}
	return mf.Name, mf, nil
}

// ConfirmModule moves the staged .raw to its permanent location, pins the
// signer SAN, writes config, updates the local desired set, and reconciles.
func (m *Manager) ConfirmModule(ctx context.Context, stageID, configTOML string) error {
	m.stagingMu.Lock()
	st, ok := m.staging[stageID]
	m.stagingMu.Unlock()
	if !ok {
		return fmt.Errorf("unknown stage id %q", stageID)
	}

	if err := CheckPinnedSAN(m.trustPath(), st.id, st.signerSAN); err != nil {
		return err
	}

	dest := rawDestPath(m.opts.RawStorageDir, st.id, st.version)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("dest dir: %w", err)
	}
	if err := os.Rename(st.rawPath, dest); err != nil {
		// cross-device fallback
		if err2 := copyFile(st.rawPath, dest); err2 != nil {
			return fmt.Errorf("move raw: %w", err2)
		}
		_ = os.Remove(st.rawPath)
	}

	// Blob is now at dest; staged source no longer exists. Clean up staging
	// state so any later error leaves a clean slate (retry must re-stage).
	m.stagingMu.Lock()
	delete(m.staging, stageID)
	m.stagingMu.Unlock()
	_ = os.RemoveAll(st.stageDir)

	if err := PinSAN(m.trustPath(), st.id, st.signerSAN); err != nil {
		return fmt.Errorf("pin san: %w", err)
	}

	if configTOML != "" {
		if err := m.writeLocalConfig(st.id, configTOML); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	m.desiredMu.Lock()
	m.localDesired = upsertModule(m.localDesired, &waypointv1.ModuleDesired{
		Id:         st.id,
		Version:    st.version,
		RawUrl:     "file://" + dest,
		ConfigToml: configTOML,
	})
	set := m.localDesired
	m.desiredMu.Unlock()

	if err := WriteLocalDesired(m.localStorePath(), set); err != nil {
		return fmt.Errorf("write local desired: %w", err)
	}

	return m.reconcileMerged(m.runCtxOr(ctx))
}

// UninstallModule removes a locally-installed module from the desired set,
// reconciles, and deletes its stored .raw.
func (m *Manager) UninstallModule(ctx context.Context, id string) error {
	m.desiredMu.Lock()
	if !containsModule(m.localDesired, id) {
		m.desiredMu.Unlock()
		return fmt.Errorf("module %q is not locally installed", id)
	}
	// capture version before removing
	var version string
	for _, mod := range m.localDesired.GetModules() {
		if mod.Id == id {
			version = mod.Version
			break
		}
	}
	m.localDesired = removeModule(m.localDesired, id)
	set := m.localDesired
	m.desiredMu.Unlock()

	if err := WriteLocalDesired(m.localStorePath(), set); err != nil {
		return fmt.Errorf("write local desired: %w", err)
	}
	if err := m.reconcileMerged(m.runCtxOr(ctx)); err != nil {
		return err
	}
	if version != "" {
		_ = os.RemoveAll(filepath.Dir(rawDestPath(m.opts.RawStorageDir, id, version)))
	}
	return nil
}

// ListModules returns the combined module status: local entries first, then
// proxy entries not shadowed by a local entry.
func (m *Manager) ListModules() []ModuleStatus {
	m.desiredMu.Lock()
	local := m.localDesired.GetModules()
	proxy := m.proxyDesired.GetModules()
	m.desiredMu.Unlock()

	seen := map[string]bool{}
	var out []ModuleStatus
	for _, mod := range local {
		out = append(out, ModuleStatus{ID: mod.Id, Version: mod.Version, Origin: "local"})
		seen[mod.Id] = true
	}
	for _, mod := range proxy {
		if seen[mod.Id] {
			continue
		}
		out = append(out, ModuleStatus{ID: mod.Id, Version: mod.Version, Origin: "proxy"})
	}
	return out
}

// writeLocalConfig extracts [modules_config.<id>] from configTOML and writes
// it as the module's runtime config.toml. Handles both the full modules.toml
// format (with [[modules]] list) and the bare [modules_config.<id>] form.
func (m *Manager) writeLocalConfig(id, configTOML string) error {
	cfgs, err := decodeModulesConfig(configTOML)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	cfg := cfgs[id]
	if cfg == nil {
		cfg = map[string]any{}
	}
	dir := filepath.Join(m.opts.RuntimeDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeTOML(filepath.Join(dir, "config.toml"), cfg)
}

// copyFile copies src to dst, creating dst if needed.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func uiKindString(k UIKind) string {
	switch k {
	case UIKindStatic:
		return "static"
	case UIKindProxy:
		return "proxy"
	default:
		return "none"
	}
}

func flattenPerms(p Permissions) []string {
	out := make([]string, 0, len(p.Publish)+len(p.Subscribe)+len(p.Hardware))
	out = append(out, p.Publish...)
	out = append(out, p.Subscribe...)
	out = append(out, p.Hardware...)
	return out
}

// hasPin reports whether the trust file has an entry for id.
func hasPin(path, id string) bool {
	tf, err := readTrust(path)
	if err != nil {
		return false
	}
	_, ok := tf.Pins[id]
	return ok
}

func newStageID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

