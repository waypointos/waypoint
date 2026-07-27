package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/waypointos/waypoint/agent/internal/localauth"
)

// RuntimePaths are the paths WriteRuntimeFiles produced. They're passed to
// the module process via environment variables.
type RuntimePaths struct {
	ConfigPath string // $WAYPOINT_MODULE_CONFIG
	CredsPath  string // $WAYPOINT_MODULE_CREDS
}

// WriteRuntimeFiles writes a module's opaque per-rover config and its
// per-process NATS credentials to /run/waypoint/modules/<id>/. Files are
// 0600 so only the agent and the module process (running as the same uid)
// can read them. Idempotent: a second call with the same inputs overwrites
// the previous files.
func WriteRuntimeFiles(runtimeDir string, spec ModuleSpec, creds localauth.Credentials) (RuntimePaths, error) {
	dir := filepath.Join(runtimeDir, spec.Manifest.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return RuntimePaths{}, fmt.Errorf("creds: mkdir %s: %w", dir, err)
	}

	configPath := filepath.Join(dir, "config.toml")
	if err := writeTOML(configPath, spec.Config); err != nil {
		return RuntimePaths{}, fmt.Errorf("creds: write config: %w", err)
	}

	credsPath := filepath.Join(dir, "creds.env")
	body := fmt.Sprintf("WAYPOINT_NATS_USER=%s\nWAYPOINT_NATS_PASSWORD=%s\n", creds.Username, creds.Password)
	if err := os.WriteFile(credsPath, []byte(body), 0o600); err != nil {
		return RuntimePaths{}, fmt.Errorf("creds: write creds: %w", err)
	}

	return RuntimePaths{ConfigPath: configPath, CredsPath: credsPath}, nil
}

func writeTOML(path string, v any) error {
	buf := new(strings.Builder)
	if err := toml.NewEncoder(buf).Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0o600)
}
