package modules

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleModulesTOML = `
[[modules]]
id     = "umr"
enable = true

[modules_config.umr]
host             = "https://192.168.105.1"
password         = "secret"
poll_interval_s  = 5

[[modules]]
id     = "future"
enable = false

[modules_config.future]
hello = "world"
`

func TestLoadModulesConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modules.toml")
	if err := os.WriteFile(path, []byte(sampleModulesTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadModulesConfig(path)
	if err != nil {
		t.Fatalf("LoadModulesConfig: %v", err)
	}
	if len(cfg.Modules) != 2 {
		t.Fatalf("want 2 modules, got %d", len(cfg.Modules))
	}
	if cfg.Modules[0].ID != "umr" || !cfg.Modules[0].Enable {
		t.Fatalf("umr: %+v", cfg.Modules[0])
	}
	if cfg.Modules[1].ID != "future" || cfg.Modules[1].Enable {
		t.Fatalf("future: %+v", cfg.Modules[1])
	}
	if cfg.Modules[0].Config["host"] != "https://192.168.105.1" {
		t.Fatalf("opaque config not preserved: %+v", cfg.Modules[0].Config)
	}
}

func TestLoadModulesConfig_MissingFileIsEmpty(t *testing.T) {
	cfg, err := LoadModulesConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("missing file should be tolerated, got: %v", err)
	}
	if len(cfg.Modules) != 0 {
		t.Fatalf("want 0 modules, got %d", len(cfg.Modules))
	}
}

func TestLoadModulesConfig_DuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modules.toml")
	dup := `
[[modules]]
id = "umr"
[[modules]]
id = "umr"
`
	if err := os.WriteFile(path, []byte(dup), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModulesConfig(path); err == nil {
		t.Fatal("want error on duplicate id")
	}
}
