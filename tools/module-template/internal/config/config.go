// Package config parses the module's per-rover config file
// (/run/waypoint/modules/example/config.toml on the rover, written by the agent
// from the operator-supplied config_toml). Define whatever fields your module
// needs here.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds the runtime configuration for the example module.
type Config struct {
	Message  string
	Interval time.Duration
}

// Load reads and validates the TOML config at path. interval_s defaults to 5
// if unset or zero.
func Load(path string) (*Config, error) {
	var raw struct {
		Message   string `toml:"message"`
		IntervalS int    `toml:"interval_s"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	interval := time.Duration(raw.IntervalS) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}
	return &Config{Message: raw.Message, Interval: interval}, nil
}
