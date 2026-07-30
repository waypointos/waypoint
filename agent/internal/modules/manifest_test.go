package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleManifestTOML = `
name        = "umr"
label       = "Connectivity"
version     = "0.1.0"
api_version = "1"
language    = "go"
entrypoint  = "waypoint-module-umr"

[permissions]
publish = [
  "waypoint.*.module.umr.stats",
  "waypoint.*.module.umr.health.ready",
]
subscribe = []
hardware  = []

[health]
probe       = "ready"
interval_s  = 5
timeout_s   = 2

[ui]
tab_id = "m-umr"
`

func TestParseManifest_Valid(t *testing.T) {
	m, err := ParseManifest([]byte(sampleManifestTOML))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Name != "umr" || m.Label != "Connectivity" || m.Version != "0.1.0" {
		t.Fatalf("identity fields: %+v", m)
	}
	if m.Entrypoint != "waypoint-module-umr" {
		t.Fatalf("entrypoint: %q", m.Entrypoint)
	}
	if len(m.Permissions.Publish) != 2 {
		t.Fatalf("publish: %+v", m.Permissions.Publish)
	}
	if m.Health.Interval != 5*time.Second {
		t.Fatalf("interval: %v", m.Health.Interval)
	}
	if m.Health.Timeout != 2*time.Second {
		t.Fatalf("timeout: %v", m.Health.Timeout)
	}
	if m.Health.Probe != "ready" {
		t.Fatalf("probe: %q", m.Health.Probe)
	}
	if m.UI.TabID != "m-umr" {
		t.Fatalf("tab_id: %q", m.UI.TabID)
	}
}

func TestParseManifest_MissingName(t *testing.T) {
	_, err := ParseManifest([]byte(`version = "0.1.0"`))
	if err == nil {
		t.Fatal("want error on missing name")
	}
}

func TestParseManifest_MissingEntrypoint(t *testing.T) {
	_, err := ParseManifest([]byte(`name = "x"` + "\n" + `version = "1"`))
	if err == nil {
		t.Fatal("want error on missing entrypoint")
	}
}

func TestParseManifest_ConfigFields(t *testing.T) {
	const m = `
name = "umr"
entrypoint = "waypoint-module-umr"

[[config.fields]]
key     = "host"
label   = "Router URL"
type    = "url"
default = "https://192.168.105.1"

[[config.fields]]
key      = "password"
label    = "Owner password"
type     = "password"
required = true
help     = "The router's local owner password."

[[config.fields]]
key  = "poll_interval_s"
type = "number"
`
	got, err := ParseManifest([]byte(m))
	require.NoError(t, err)
	require.Len(t, got.ConfigFields, 3)
	require.Equal(t, ConfigField{
		Key: "host", Label: "Router URL", Type: "url", Default: "https://192.168.105.1",
	}, got.ConfigFields[0])
	require.True(t, got.ConfigFields[1].Required)
	require.Equal(t, "The router's local owner password.", got.ConfigFields[1].Help)
	// Order is the manifest's: a form reads top to bottom.
	require.Equal(t, "poll_interval_s", got.ConfigFields[2].Key)
	require.Equal(t, "poll_interval_s", got.ConfigFields[2].Label, "label defaults to the key")
}

func TestParseManifest_ConfigFieldsRejected(t *testing.T) {
	base := "name = \"x\"\nentrypoint = \"e\"\n"
	for name, doc := range map[string]string{
		"bad key":        base + "\n[[config.fields]]\nkey = \"Host Name\"\n",
		"empty key":      base + "\n[[config.fields]]\nlabel = \"No key\"\n",
		"duplicate keys": base + "\n[[config.fields]]\nkey = \"host\"\n\n[[config.fields]]\nkey = \"host\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseManifest([]byte(doc))
			require.Error(t, err)
		})
	}
}

// An unrecognised type must survive parsing: the module may be newer than the
// dashboard, which falls back to a text input.
func TestParseManifest_ConfigFieldUnknownTypeKept(t *testing.T) {
	doc := "name = \"x\"\nentrypoint = \"e\"\n\n[[config.fields]]\nkey = \"colour\"\ntype = \"colour-picker\"\n"
	got, err := ParseManifest([]byte(doc))
	require.NoError(t, err)
	require.Equal(t, "colour-picker", got.ConfigFields[0].Type)
}

func TestParseManifest_UnknownKeysIgnored(t *testing.T) {
	doc := sampleManifestTOML + "\nfuture_field = \"someday\"\n"
	if _, err := ParseManifest([]byte(doc)); err != nil {
		t.Fatalf("unknown keys must be ignored, got: %v", err)
	}
}

func TestParseManifest_UIStatic(t *testing.T) {
	const m = `
name = "power-monitor"
entrypoint = "waypoint-module-power-monitor"
[ui.static]
tab_id = "m-power-monitor"
bundle = "/dashboard/panel.js"
`
	got, err := ParseManifest([]byte(m))
	require.NoError(t, err)
	require.Equal(t, UIKindStatic, got.UI.Kind)
	require.Equal(t, "m-power-monitor", got.UI.TabID)
	require.Equal(t, "/dashboard/panel.js", got.UI.StaticBundle)
}

func TestParseManifest_UIProxy(t *testing.T) {
	const m = `
name = "power-monitor"
entrypoint = "waypoint-module-power-monitor"
[ui.proxy]
tab_id = "m-power-monitor"
port = 8081
lan_only = true
`
	got, err := ParseManifest([]byte(m))
	require.NoError(t, err)
	require.Equal(t, UIKindProxy, got.UI.Kind)
	require.Equal(t, uint16(8081), got.UI.ProxyPort)
	require.True(t, got.UI.LANOnly)
}

func TestParseManifest_UIStaticAndProxyRejected(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[ui.static]
tab_id = "m-x"
bundle = "/dashboard/panel.js"
[ui.proxy]
tab_id = "m-x"
port = 8081
`
	_, err := ParseManifest([]byte(m))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestParseManifest_UIProxyRequiresLANOnly(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[ui.proxy]
tab_id = "m-x"
port = 8081
lan_only = false
`
	_, err := ParseManifest([]byte(m))
	require.Error(t, err)
	require.Contains(t, err.Error(), "lan_only must be true")
}

func TestParseManifest_Hardware(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[hardware]
devices    = ["/dev/i2c-1", "/dev/spidev0.0"]
read_only  = ["/sys/class/hwmon"]
capabilities = ["NET_ADMIN"]
`
	got, err := ParseManifest([]byte(m))
	require.NoError(t, err)
	require.Equal(t, []string{"/dev/i2c-1", "/dev/spidev0.0"}, got.Hardware.Devices)
	require.Equal(t, []string{"/sys/class/hwmon"}, got.Hardware.ReadOnly)
	require.Equal(t, []string{"NET_ADMIN"}, got.Hardware.Capabilities)
}

func TestParseManifest_HardwarePathDisallowed(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[hardware]
read_only = ["/etc/shadow"]
`
	_, err := ParseManifest([]byte(m))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in allow-list")
}

func TestParseManifest_HardwareCapabilityDisallowed(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[hardware]
capabilities = ["SYS_ADMIN"]
`
	_, err := ParseManifest([]byte(m))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in allow-list")
}

// /dev/ttyAMA0 is the ESP32/core servo byte pipe; no module may be granted a
// Pi hardware UART. Drive-affecting actions go through NATS, not raw serial.
func TestParseManifest_RejectsESP32UART(t *testing.T) {
	for _, dev := range []string{"/dev/ttyAMA0", "/dev/ttyAMA1"} {
		const tmpl = "name = \"x\"\nentrypoint = \"x\"\n[hardware]\ndevices = [%q]\n"
		_, err := ParseManifest([]byte(fmt.Sprintf(tmpl, dev)))
		require.Error(t, err, "device %q must be rejected", dev)
		require.Contains(t, err.Error(), "not in allow-list")
	}
}

// USB serial stays allowed (e.g. the power-monitor module on /dev/ttyACM0).
func TestParseManifest_AllowsUSBSerial(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[hardware]
devices = ["/dev/ttyACM0", "/dev/ttyUSB0"]
`
	got, err := ParseManifest([]byte(m))
	require.NoError(t, err)
	require.Equal(t, []string{"/dev/ttyACM0", "/dev/ttyUSB0"}, got.Hardware.Devices)
}

func TestParseManifest_HardwareReadWriteAllowed(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[hardware]
read_write = ["/var/lib/waypoint-module-x"]
`
	got, err := ParseManifest([]byte(m))
	require.NoError(t, err)
	require.Equal(t, []string{"/var/lib/waypoint-module-x"}, got.Hardware.ReadWrite)
}

func TestParseManifest_HardwareReadWriteDisallowed(t *testing.T) {
	const m = `
name = "x"
entrypoint = "x"
[hardware]
read_write = ["/etc/shadow"]
`
	_, err := ParseManifest([]byte(m))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in allow-list")
}

func TestParseManifest_AllowsTtyACMDevice(t *testing.T) {
	m, err := ParseManifest([]byte(`
name = "power-monitor"
entrypoint = "waypoint-module-power-monitor"
[hardware]
devices = ["/dev/ttyACM0"]
`))
	require.NoError(t, err)
	require.Equal(t, []string{"/dev/ttyACM0"}, m.Hardware.Devices)
}

func TestParseManifest_RejectsUnlistedDevice(t *testing.T) {
	_, err := ParseManifest([]byte(`
name = "x"
entrypoint = "x"
[hardware]
devices = ["/dev/mem"]
`))
	require.Error(t, err)
}

func TestParseManifest_RejectsBadName(t *testing.T) {
	for _, bad := range []string{"../../..", "Foo", "a/b", "with space", "under_score", ""} {
		_, err := ParseManifest([]byte("name=\"" + bad + "\"\nentrypoint=\"x\"\n"))
		require.Error(t, err, "name %q should be rejected", bad)
	}
	_, err := ParseManifest([]byte("name=\"power-monitor\"\nentrypoint=\"x\"\n"))
	require.NoError(t, err)
}

func TestParseManifest_RejectsOutOfSandboxSubjects(t *testing.T) {
	for _, bad := range []string{
		"waypoint.*.cmd.drive",
		"waypoint.*.telemetry.system",
		"waypoint.*.module.other.stats",
		"waypoint.*.telemetry.uplink",
		"waypoint.*.module.umr.",   // bare prefix, empty trailing token (invalid NATS subject)
		"waypoint.*.module.umrX.stats", // different module via prefix collision
	} {
		doc := "name=\"umr\"\nentrypoint=\"x\"\n[permissions]\npublish=[" + fmt.Sprintf("%q", bad) + "]\n"
		_, err := ParseManifest([]byte(doc))
		require.Error(t, err, "publish %q must be rejected", bad)
		require.Contains(t, err.Error(), "outside module sandbox")
	}
}

func TestParseManifest_RejectsOutOfSandboxSubscribe(t *testing.T) {
	doc := "name=\"umr\"\nentrypoint=\"x\"\n[permissions]\nsubscribe=[\"waypoint.*.telemetry.system\"]\n"
	_, err := ParseManifest([]byte(doc))
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside module sandbox")
}

func TestParseManifest_AllowsOwnSandboxSubjects(t *testing.T) {
	doc := `name = "umr"
entrypoint = "x"
[permissions]
publish   = ["waypoint.*.module.umr.stats", "waypoint.*.module.umr.uplink"]
subscribe = ["waypoint.*.module.umr.health.ready", "waypoint.*.module.umr.>"]
`
	_, err := ParseManifest([]byte(doc))
	require.NoError(t, err)
}

func TestParseManifest_ProvidesParsedAndValidated(t *testing.T) {
	ok := `name = "umr"
entrypoint = "x"
provides = ["uplink"]
`
	m, err := ParseManifest([]byte(ok))
	require.NoError(t, err)
	require.Equal(t, []string{"uplink"}, m.Provides)

	_, err = ParseManifest([]byte("name=\"umr\"\nentrypoint=\"x\"\nprovides=[\"telemetry\"]\n"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown capability")
}

func TestParseManifest_RequiresServoControl(t *testing.T) {
	m, err := ParseManifest([]byte(`
name        = "so100"
entrypoint  = "waypoint-module-so100"
requires    = ["servo-control"]
`))
	require.NoError(t, err)
	require.Equal(t, []string{"servo-control"}, m.Requires)
}

func TestParseManifest_RejectsUnknownRequires(t *testing.T) {
	_, err := ParseManifest([]byte(`
name       = "so100"
entrypoint = "waypoint-module-so100"
requires   = ["root-everything"]
`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown required capability")
}

func TestParseManifest_TeleopInputAndWindow(t *testing.T) {
	toml := `
name = "so100"
label = "Arm"
version = "0.1.0"
api_version = "1"
language = "go"
entrypoint = "waypoint-module-so100"
requires = ["servo-control", "teleop-input"]

[ui.static]
tab_id = "m-so100"

[ui.teleop]
window_id = "w-so100"
label = "Arm"
entry = "teleop.js"
bindings = ["right_stick", "triggers"]
`
	m, err := ParseManifest([]byte(toml))
	require.NoError(t, err)
	require.Contains(t, m.Requires, "teleop-input")
	require.NotNil(t, m.UI.Teleop)
	require.Equal(t, "w-so100", m.UI.Teleop.WindowID)
	require.Equal(t, "teleop.js", m.UI.Teleop.Entry)
	require.Equal(t, []string{"right_stick", "triggers"}, m.UI.Teleop.Bindings)
}

func TestManifestComponentParsed(t *testing.T) {
	m, err := ParseManifest([]byte(`
name = "myarm"
entrypoint = "waypoint-module-myarm"
[component]
class = "arm"
state_rate_hz = 20
`))
	require.NoError(t, err)
	require.Len(t, m.Components, 1)
	assert.Equal(t, "arm", m.Components[0].Class)
	assert.Equal(t, 20.0, m.Components[0].StateRateHz)
}

func TestManifestComponentDefaultsRate(t *testing.T) {
	m, err := ParseManifest([]byte(`
name = "myarm"
entrypoint = "x"
[component]
class = "sensor"
`))
	require.NoError(t, err)
	require.Len(t, m.Components, 1)
	assert.Equal(t, 10.0, m.Components[0].StateRateHz)
}

func TestManifestComponentRejectsMalformedClass(t *testing.T) {
	for _, class := range []string{"Drill", "x", "3dprinter", strings.Repeat("a", 33)} {
		_, err := ParseManifest([]byte("name = \"m\"\nentrypoint = \"x\"\n[component]\nclass = \"" + class + "\"\n"))
		require.Error(t, err, "class %q", class)
		assert.Contains(t, err.Error(), "must match [a-z][a-z0-9-]{1,31}")
	}
}

func TestManifestComponentAcceptsGenericClass(t *testing.T) {
	m, err := ParseManifest([]byte(`
name = "mydrill"
entrypoint = "x"
[component]
class = "drill"
`))
	require.NoError(t, err)
	require.Len(t, m.Components, 1)
	assert.Equal(t, "drill", m.Components[0].Class)
	pub, sub := componentLeaves(m.Name, m.Components[0].Class)
	assert.Equal(t, []string{"waypoint.*.module.mydrill.drill.state"}, pub)
	assert.Equal(t, []string{"waypoint.*.module.mydrill.drill.cmd"}, sub)
}

func TestManifestComponentRejectsBadRate(t *testing.T) {
	_, err := ParseManifest([]byte("name = \"m\"\nentrypoint = \"x\"\n[component]\nclass = \"arm\"\nstate_rate_hz = 250\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state_rate_hz")
}

func TestManifestComponentsArrayForm(t *testing.T) {
	m, err := ParseManifest([]byte(`
name = "drill"
entrypoint = "waypoint-module-drill"

[[components]]
class = "drill"
state_rate_hz = 20

[[components]]
class = "sensor"
state_rate_hz = 10
`))
	require.NoError(t, err)
	require.Len(t, m.Components, 2)
	assert.Equal(t, Component{Class: "drill", StateRateHz: 20}, m.Components[0])
	assert.Equal(t, Component{Class: "sensor", StateRateHz: 10}, m.Components[1])
}

func TestManifestComponentsArrayDefaultsRate(t *testing.T) {
	m, err := ParseManifest([]byte("name = \"m\"\nentrypoint = \"x\"\n[[components]]\nclass = \"sensor\"\n"))
	require.NoError(t, err)
	require.Len(t, m.Components, 1)
	assert.Equal(t, 10.0, m.Components[0].StateRateHz)
}

func TestManifestComponentsBothFormsRejected(t *testing.T) {
	_, err := ParseManifest([]byte(`
name = "m"
entrypoint = "x"

[component]
class = "arm"

[[components]]
class = "sensor"
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestManifestComponentsDuplicateClassRejected(t *testing.T) {
	_, err := ParseManifest([]byte(`
name = "m"
entrypoint = "x"

[[components]]
class = "sensor"

[[components]]
class = "sensor"
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate component class")
}
