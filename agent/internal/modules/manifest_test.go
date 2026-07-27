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
	require.NotNil(t, m.Component)
	assert.Equal(t, "arm", m.Component.Class)
	assert.Equal(t, 20.0, m.Component.StateRateHz)
}

func TestManifestComponentDefaultsRate(t *testing.T) {
	m, err := ParseManifest([]byte(`
name = "myarm"
entrypoint = "x"
[component]
class = "sensor"
`))
	require.NoError(t, err)
	assert.Equal(t, 10.0, m.Component.StateRateHz)
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
	require.NotNil(t, m.Component)
	assert.Equal(t, "drill", m.Component.Class)
	pub, sub := componentLeaves(m.Name, m.Component.Class)
	assert.Equal(t, []string{"waypoint.*.module.mydrill.drill.state"}, pub)
	assert.Equal(t, []string{"waypoint.*.module.mydrill.drill.cmd"}, sub)
}

func TestManifestComponentRejectsBadRate(t *testing.T) {
	_, err := ParseManifest([]byte("name = \"m\"\nentrypoint = \"x\"\n[component]\nclass = \"arm\"\nstate_rate_hz = 250\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state_rate_hz")
}
