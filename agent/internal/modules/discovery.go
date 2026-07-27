package modules

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// ModuleSpec couples an installed manifest with this rover's per-rover
// configuration. Discover produces one ModuleSpec per module that is both
// installed and enabled.
type ModuleSpec struct {
	Manifest Manifest
	Config   map[string]any
}

// Discover walks installDir for module.toml manifests and intersects them
// with the rover's modules.toml. Unknown ids in the config are logged and
// skipped (a rover may have stale config after a module is uninstalled).
func Discover(installDir, configPath string) ([]ModuleSpec, error) {
	manifests, err := readManifests(installDir)
	if err != nil {
		return nil, err
	}
	rover, err := LoadModulesConfig(configPath)
	if err != nil {
		return nil, err
	}
	out := make([]ModuleSpec, 0, len(rover.Modules))
	for _, r := range rover.Modules {
		if !r.Enable {
			continue
		}
		m, ok := manifests[r.ID]
		if !ok {
			// No baked manifest. Expected for a runtime-installed module (it
			// attaches via the reconciler from modules.local.pb, not from here).
			slog.Info(fmt.Sprintf("modules: %q in modules.toml has no baked manifest; skipping baked spec", r.ID))
			continue
		}
		out = append(out, ModuleSpec{Manifest: *m, Config: r.Config})
	}
	return out, nil
}

func readManifests(installDir string) (map[string]*Manifest, error) {
	out := map[string]*Manifest{}
	entries, err := os.ReadDir(installDir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("modules: read install dir %q: %w", installDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(installDir, e.Name(), "module.toml")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("modules: read %q: %w", path, err)
		}
		m, err := ParseManifest(data)
		if err != nil {
			return nil, fmt.Errorf("modules: %q: %w", path, err)
		}
		out[m.Name] = m
	}
	return out, nil
}
