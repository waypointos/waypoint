package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_IntersectsInstalledAndEnabled(t *testing.T) {
	installDir := t.TempDir()
	configDir := t.TempDir()

	mustWrite(t, filepath.Join(installDir, "umr", "module.toml"), sampleManifestTOML)
	mustWrite(t, filepath.Join(installDir, "vision", "module.toml"), `
name = "vision"
version = "0.1.0"
entrypoint = "waypoint-module-vision"
`)

	mustWrite(t, filepath.Join(configDir, "modules.toml"), `
[[modules]]
id = "umr"
enable = true

[[modules]]
id = "vision"
enable = false

[[modules]]
id = "ghost"
enable = true
`)

	specs, err := Discover(installDir, filepath.Join(configDir, "modules.toml"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("want 1 spec (umr only), got %d: %+v", len(specs), specs)
	}
	if specs[0].Manifest.Name != "umr" {
		t.Fatalf("got: %+v", specs[0])
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
