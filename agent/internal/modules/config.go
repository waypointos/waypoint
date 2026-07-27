package modules

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// RoverModuleConfig is one [[modules]] entry from modules.toml, joined
// with its sibling [modules_config.<id>] table.
type RoverModuleConfig struct {
	ID     string
	Enable bool
	Config map[string]any
}

// RoverModulesConfig is the full parsed modules.toml.
type RoverModulesConfig struct {
	Modules []RoverModuleConfig
}

// LoadModulesConfig reads modules.toml. A missing file is treated as
// "no modules enabled" — not an error.
func LoadModulesConfig(path string) (*RoverModulesConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &RoverModulesConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("modules.toml: read: %w", err)
	}
	return ParseModulesConfigString(string(data))
}

// decodeModulesConfig decodes the [modules_config.<id>] tables from a TOML
// string into a per-id map. Shared by writeLocalConfig so both parse the same way.
func decodeModulesConfig(s string) (map[string]map[string]any, error) {
	var raw struct {
		ModulesConfig map[string]map[string]any `toml:"modules_config"`
	}
	if _, err := toml.NewDecoder(strings.NewReader(s)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("modules config: decode: %w", err)
	}
	return raw.ModulesConfig, nil
}

// ParseModulesConfigString decodes a modules.toml document.
//
// Schema uses two top-level keys to avoid the TOML dual-namespace
// conflict that arises when [[modules]] and [modules.<id>.config]
// share the "modules" key:
//
//	[[modules]]
//	id     = "umr"
//	enable = true
//
//	[modules_config.umr]
//	host = "https://192.168.105.1"
func ParseModulesConfigString(s string) (*RoverModulesConfig, error) {
	var raw struct {
		Modules []struct {
			ID     string `toml:"id"`
			Enable bool   `toml:"enable"`
		} `toml:"modules"`
		ModulesConfig map[string]map[string]any `toml:"modules_config"`
	}
	if _, err := toml.NewDecoder(strings.NewReader(s)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("modules.toml: decode: %w", err)
	}
	cfg := &RoverModulesConfig{Modules: make([]RoverModuleConfig, 0, len(raw.Modules))}
	seen := map[string]struct{}{}
	for i, m := range raw.Modules {
		if m.ID == "" {
			return nil, fmt.Errorf("modules.toml: modules[%d].id is required", i)
		}
		if _, dup := seen[m.ID]; dup {
			return nil, fmt.Errorf("modules.toml: duplicate module id %q", m.ID)
		}
		seen[m.ID] = struct{}{}
		cfg.Modules = append(cfg.Modules, RoverModuleConfig{
			ID:     m.ID,
			Enable: m.Enable,
			Config: raw.ModulesConfig[m.ID],
		})
	}
	return cfg, nil
}
