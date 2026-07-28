// Package modules implements the agent-side module system: parsing
// manifests, resolving per-rover enablement, minting per-module NATS
// users, supervising module processes, and publishing the active-module
// snapshot for the dashboard.
package modules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Manifest is the parsed shape of a module's module.toml file. The file
// is image-baked: every rover that has the module installed sees the same
// manifest. Per-rover enablement and config live in modules.toml instead.
type Manifest struct {
	Name        string
	Label       string
	Version     string
	APIVersion  string
	Language    string
	Entrypoint  string
	Permissions Permissions
	Health      HealthProbe
	UI          UIBinding
	Hardware    Hardware
	Provides    []string
	Requires    []string
	Component   *Component
	// ConfigFields is the manifest [[config.fields]] schema for the module's
	// per-rover config.toml. Empty means the module declares none, and the
	// dashboard falls back to a raw TOML editor.
	ConfigFields []ConfigField
}

// ConfigField is one editable setting in a module's per-rover config.
type ConfigField struct {
	Key      string
	Label    string
	Type     string // text | url | password | number | bool; empty defaults to text
	Default  string
	Help     string
	Required bool
}

// Component is the manifest [component] declaration: the standard class this
// module serves. Declaring it auto-grants the class's standard sandbox leaves.
type Component struct {
	Class       string
	StateRateHz float64
}

type Permissions struct {
	Publish   []string
	Subscribe []string
	Hardware  []string
}

type HealthProbe struct {
	Probe    string
	Interval time.Duration
	Timeout  time.Duration
}

type UIKind int

const (
	UIKindNone UIKind = iota
	UIKindStatic
	UIKindProxy
)

type UIBinding struct {
	Kind         UIKind
	TabID        string
	StaticBundle string // path inside .raw, e.g. "/dashboard/panel.js"
	ProxyPort    uint16
	LANOnly      bool
	Teleop       *TeleopWindowBinding
}

// TeleopWindowBinding describes a module-provided draggable teleop window,
// declared via [ui.teleop]. It is independent of the static/proxy tab kind.
type TeleopWindowBinding struct {
	WindowID string
	Label    string
	Entry    string
	Bindings []string
}

type Hardware struct {
	Devices      []string
	ReadOnly     []string
	ReadWrite    []string
	Capabilities []string
}

var moduleNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func validModuleName(s string) bool { return moduleNameRe.MatchString(s) }

var (
	// /dev/ttyAMA* (the Pi hardware UARTs) are deliberately excluded: ttyAMA0 is the
	// ESP32 servo/relay byte pipe owned by core and is safety-critical. Modules must
	// never get raw UART access to it; drive-affecting actions route through NATS.
	// Module serial use is USB only (ttyUSB/ttyACM).
	allowedDevicePrefixes = []string{
		"/dev/i2c-", "/dev/spidev", "/dev/ttyUSB", "/dev/ttyACM",
		"/dev/video", "/dev/gpiochip",
	}
	allowedReadOnlyPaths = []string{
		"/sys/class/hwmon", "/sys/class/gpio", "/sys/class/thermal",
	}
	allowedReadWritePaths = []string{
		"/var/lib/waypoint-module-", // module-private state, must be prefixed with module name
	}
	allowedCapabilities = map[string]struct{}{
		"NET_ADMIN": {}, "SYS_NICE": {},
	}
)

// knownCapabilities are the platform capabilities a module may declare via
// `provides`. Each lets the agent bridge the module to a specific platform
// subject (e.g. "uplink" → telemetry.uplink); the module itself stays sandboxed.
var knownCapabilities = map[string]struct{}{"uplink": {}}

// knownRequires are privileged platform capabilities a module may declare via
// `requires`. Unlike `provides` (outbound), these grant the module agent-brokered
// access INTO a platform/core subject, so they are admin-gated (a module that
// declares one only attaches when admin-registered through the signed-module
// trust path). "servo-control" lets the agent bridge module.<id>.servo.* to
// core's cmd.servo / rpc.servo_read.
var knownRequires = map[string]struct{}{"servo-control": {}, "teleop-input": {}}

// componentClassRe bounds the open component-class registry: any well-formed
// class name is accepted, typed classes are just the ones with a schema.
var componentClassRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)

// componentLeaves returns the standard sandbox leaves for a class, manifest
// wildcard form. Sensor is read-only: no cmd leaf. Any other class gets the
// generic state/cmd pair.
func componentLeaves(name, class string) (publish, subscribe []string) {
	base := "waypoint.*.module." + name + "."
	switch class {
	case "arm":
		return []string{base + "arm.state"}, []string{base + "arm.cmd"}
	case "sensor":
		return []string{base + "sensor.state"}, nil
	case "base":
		return []string{base + "base.state"}, []string{base + "base.cmd"}
	}
	return []string{base + class + ".state"}, []string{base + class + ".cmd"}
}

// subjectInOwnSandbox reports whether subj is within the module's own subtree.
// Module subjects use a literal "*" for the rover id; anything outside
// waypoint.*.module.<name>.* is rejected so a manifest cannot reach cmd.*,
// telemetry.*, rpc.*, or another module's subtree.
func subjectInOwnSandbox(name, subj string) bool {
	prefix := "waypoint.*.module." + name + "."
	return strings.HasPrefix(subj, prefix) && len(subj) > len(prefix)
}

// ParseManifest reads a module.toml byte slice. Unknown keys are ignored
// (forward-compatible across module-system versions).
func ParseManifest(data []byte) (*Manifest, error) {
	var raw rawManifest
	if _, err := toml.NewDecoder(strings.NewReader(string(data))).Decode(&raw); err != nil {
		return nil, fmt.Errorf("manifest: decode: %w", err)
	}
	if raw.Name == "" {
		return nil, errors.New("manifest: name is required")
	}
	if !validModuleName(raw.Name) {
		return nil, fmt.Errorf("manifest: name %q must match [a-z][a-z0-9-]{0,63}", raw.Name)
	}
	if raw.Entrypoint == "" {
		return nil, errors.New("manifest: entrypoint is required")
	}
	for _, subj := range append(append([]string{}, raw.Permissions.Publish...), raw.Permissions.Subscribe...) {
		if !subjectInOwnSandbox(raw.Name, subj) {
			return nil, fmt.Errorf("manifest: subject %q is outside module sandbox waypoint.*.module.%s.*", subj, raw.Name)
		}
	}
	for _, cap := range raw.Provides {
		if _, ok := knownCapabilities[cap]; !ok {
			return nil, fmt.Errorf("manifest: unknown capability %q in provides", cap)
		}
	}
	for _, cap := range raw.Requires {
		if _, ok := knownRequires[cap]; !ok {
			return nil, fmt.Errorf("manifest: unknown required capability %q in requires", cap)
		}
	}
	m := &Manifest{
		Name:       raw.Name,
		Label:      raw.Label,
		Version:    raw.Version,
		APIVersion: raw.APIVersion,
		Language:   raw.Language,
		Entrypoint: raw.Entrypoint,
		Permissions: Permissions{
			Publish:   raw.Permissions.Publish,
			Subscribe: raw.Permissions.Subscribe,
			Hardware:  raw.Permissions.Hardware,
		},
		Health: HealthProbe{
			Probe:    raw.Health.Probe,
			Interval: time.Duration(raw.Health.IntervalS) * time.Second,
			Timeout:  time.Duration(raw.Health.TimeoutS) * time.Second,
		},
		Provides: raw.Provides,
		Requires: raw.Requires,
	}
	if raw.Component != nil {
		if !componentClassRe.MatchString(raw.Component.Class) {
			return nil, fmt.Errorf("manifest: component class %q must match [a-z][a-z0-9-]{1,31}", raw.Component.Class)
		}
		rate := raw.Component.StateRateHz
		if rate == 0 {
			rate = 10
		}
		if rate < 0 || rate > 100 {
			return nil, fmt.Errorf("manifest: component state_rate_hz %v out of range (0, 100]", raw.Component.StateRateHz)
		}
		m.Component = &Component{Class: raw.Component.Class, StateRateHz: rate}
	}
	fields, err := buildConfigFields(&raw)
	if err != nil {
		return nil, err
	}
	m.ConfigFields = fields
	if err := buildUI(&raw, m); err != nil {
		return nil, err
	}
	if err := buildHardware(&raw, m); err != nil {
		return nil, err
	}
	return m, nil
}

var configFieldKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// buildConfigFields validates [[config.fields]]. The type string is left
// unchecked on purpose: a module may declare a type an older dashboard predates,
// and the dashboard renders anything it does not recognise as text.
func buildConfigFields(raw *rawManifest) ([]ConfigField, error) {
	if len(raw.Config.Fields) == 0 {
		return nil, nil
	}
	out := make([]ConfigField, 0, len(raw.Config.Fields))
	seen := map[string]bool{}
	for i, f := range raw.Config.Fields {
		if !configFieldKeyRe.MatchString(f.Key) {
			return nil, fmt.Errorf("manifest: config.fields[%d].key %q must match [a-z][a-z0-9_]{0,63}", i, f.Key)
		}
		if seen[f.Key] {
			return nil, fmt.Errorf("manifest: duplicate config.fields key %q", f.Key)
		}
		seen[f.Key] = true
		label := f.Label
		if label == "" {
			label = f.Key
		}
		out = append(out, ConfigField{
			Key: f.Key, Label: label, Type: f.Type,
			Default: f.Default, Help: f.Help, Required: f.Required,
		})
	}
	return out, nil
}

func buildUI(raw *rawManifest, m *Manifest) error {
	if raw.UI.Static != nil && raw.UI.Proxy != nil {
		return errors.New("manifest: [ui.static] and [ui.proxy] are mutually exclusive")
	}
	switch {
	case raw.UI.Static != nil:
		m.UI = UIBinding{
			Kind:         UIKindStatic,
			TabID:        raw.UI.Static.TabID,
			StaticBundle: raw.UI.Static.Bundle,
		}
	case raw.UI.Proxy != nil:
		if !raw.UI.Proxy.LANOnly {
			return errors.New("manifest: [ui.proxy].lan_only must be true in v1")
		}
		if raw.UI.Proxy.Port < 1 || raw.UI.Proxy.Port > 65535 {
			return fmt.Errorf("manifest: [ui.proxy].port %d out of range", raw.UI.Proxy.Port)
		}
		m.UI = UIBinding{
			Kind:      UIKindProxy,
			TabID:     raw.UI.Proxy.TabID,
			ProxyPort: uint16(raw.UI.Proxy.Port),
			LANOnly:   true,
		}
	case raw.UI.TabID != "":
		m.UI = UIBinding{Kind: UIKindStatic, TabID: raw.UI.TabID}
	}
	if m.UI.Kind != UIKindNone && !strings.HasPrefix(m.UI.TabID, "m-") {
		return fmt.Errorf("manifest: ui.tab_id %q must start with m- (reserved prefix)", m.UI.TabID)
	}
	if raw.UI.Teleop != nil {
		m.UI.Teleop = &TeleopWindowBinding{
			WindowID: raw.UI.Teleop.WindowID,
			Label:    raw.UI.Teleop.Label,
			Entry:    raw.UI.Teleop.Entry,
			Bindings: raw.UI.Teleop.Bindings,
		}
	}
	return nil
}

func buildHardware(raw *rawManifest, m *Manifest) error {
	for _, d := range raw.Hardware.Devices {
		ok := false
		for _, prefix := range allowedDevicePrefixes {
			if strings.HasPrefix(d, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("manifest: device %q not in allow-list", d)
		}
	}
	for _, p := range raw.Hardware.ReadOnly {
		ok := false
		for _, allowed := range allowedReadOnlyPaths {
			if strings.HasPrefix(p, allowed) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("manifest: read_only path %q not in allow-list", p)
		}
	}
	for _, p := range raw.Hardware.ReadWrite {
		ok := false
		for _, allowed := range allowedReadWritePaths {
			if strings.HasPrefix(p, allowed) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("manifest: read_write path %q not in allow-list", p)
		}
	}
	for _, c := range raw.Hardware.Capabilities {
		if _, ok := allowedCapabilities[c]; !ok {
			return fmt.Errorf("manifest: capability %q not in allow-list", c)
		}
	}
	m.Hardware = Hardware{
		Devices:      raw.Hardware.Devices,
		ReadOnly:     raw.Hardware.ReadOnly,
		ReadWrite:    raw.Hardware.ReadWrite,
		Capabilities: raw.Hardware.Capabilities,
	}
	return nil
}

type rawManifest struct {
	Name        string `toml:"name"`
	Label       string `toml:"label"`
	Version     string `toml:"version"`
	APIVersion  string `toml:"api_version"`
	Language    string `toml:"language"`
	Entrypoint  string `toml:"entrypoint"`
	Permissions struct {
		Publish   []string `toml:"publish"`
		Subscribe []string `toml:"subscribe"`
		Hardware  []string `toml:"hardware"`
	} `toml:"permissions"`
	Health struct {
		Probe     string `toml:"probe"`
		IntervalS int    `toml:"interval_s"`
		TimeoutS  int    `toml:"timeout_s"`
	} `toml:"health"`
	UI struct {
		TabID  string `toml:"tab_id"`
		Static *struct {
			TabID  string `toml:"tab_id"`
			Bundle string `toml:"bundle"`
		} `toml:"static"`
		Proxy *struct {
			TabID   string `toml:"tab_id"`
			Port    int    `toml:"port"`
			LANOnly bool   `toml:"lan_only"`
		} `toml:"proxy"`
		Teleop *struct {
			WindowID string   `toml:"window_id"`
			Label    string   `toml:"label"`
			Entry    string   `toml:"entry"`
			Bindings []string `toml:"bindings"`
		} `toml:"teleop"`
	} `toml:"ui"`
	Hardware struct {
		Devices      []string `toml:"devices"`
		ReadOnly     []string `toml:"read_only"`
		ReadWrite    []string `toml:"read_write"`
		Capabilities []string `toml:"capabilities"`
	} `toml:"hardware"`
	Provides  []string `toml:"provides"`
	Requires  []string `toml:"requires"`
	Component *struct {
		Class       string  `toml:"class"`
		StateRateHz float64 `toml:"state_rate_hz"`
	} `toml:"component"`
	Config struct {
		Fields []struct {
			Key      string `toml:"key"`
			Label    string `toml:"label"`
			Type     string `toml:"type"`
			Default  string `toml:"default"`
			Help     string `toml:"help"`
			Required bool   `toml:"required"`
		} `toml:"fields"`
	} `toml:"config"`
}
