package wpmodule

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// LoadConfig decodes the module's config.toml (WAYPOINT_MODULE_CONFIG) into v.
// Returns false (no error) when the env var is unset, so dev runs without a
// config file work.
func LoadConfig(v any) (bool, error) {
	path := os.Getenv("WAYPOINT_MODULE_CONFIG")
	if path == "" {
		return false, nil
	}
	if _, err := toml.DecodeFile(path, v); err != nil {
		return false, fmt.Errorf("wpmodule: config %s: %w", path, err)
	}
	return true, nil
}
