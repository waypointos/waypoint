package modules

import (
	"fmt"
	"strconv"
	"strings"
)

// RenderHardwareDropin returns the contents of the .d/30-hardware.conf
// drop-in for a module's [hardware] block. Returns "" when nothing is
// declared so the caller can skip writing a file.
//
// rwSource, when non-nil, maps a read_write destination path to a writable
// host source so the bind reads "BindPaths=<source>:<dest>". This backs the
// module's declared (read-only-rootfs) path with a writable /data dir. When
// nil, read_write paths bind to themselves (legacy / writable-rootfs hosts).
func RenderHardwareDropin(h Hardware, rwSource func(string) string) string {
	if len(h.Devices) == 0 && len(h.ReadOnly) == 0 && len(h.ReadWrite) == 0 && len(h.Capabilities) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Service]\n")
	for _, d := range h.Devices {
		b.WriteString("DeviceAllow=")
		b.WriteString(d)
		b.WriteString(" rw\n")
	}
	for _, p := range h.ReadOnly {
		b.WriteString("BindReadOnlyPaths=")
		b.WriteString(p)
		b.WriteString("\n")
	}
	for _, p := range h.ReadWrite {
		b.WriteString("BindPaths=")
		if rwSource != nil {
			b.WriteString(rwSource(p))
			b.WriteString(":")
		}
		b.WriteString(p)
		b.WriteString("\n")
	}
	for _, c := range h.Capabilities {
		b.WriteString("AmbientCapabilities=CAP_")
		b.WriteString(c)
		b.WriteString("\n")
		b.WriteString("CapabilityBoundingSet=CAP_")
		b.WriteString(c)
		b.WriteString("\n")
	}
	return b.String()
}

// RenderComponentDropin returns the .d/40-component.conf contents exporting
// component classes and state rates to the module process. The first
// component also fills the legacy unsuffixed vars so pre-multi-component
// SDKs keep working. "" when the module declares no components.
func RenderComponentDropin(cs []Component) string {
	if len(cs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Service]\n")
	fmt.Fprintf(&b, "Environment=WAYPOINT_MODULE_COMPONENT=%s\n", cs[0].Class)
	fmt.Fprintf(&b, "Environment=WAYPOINT_MODULE_STATE_RATE_HZ=%s\n", formatRate(cs[0].StateRateHz))
	for _, c := range cs {
		fmt.Fprintf(&b, "Environment=WAYPOINT_MODULE_STATE_RATE_HZ_%s=%s\n", classEnvSuffix(c.Class), formatRate(c.StateRateHz))
	}
	return b.String()
}

func formatRate(hz float64) string { return strconv.FormatFloat(hz, 'f', -1, 64) }

// classEnvSuffix maps a component class to its env-var suffix: upper-case,
// "-" becomes "_".
func classEnvSuffix(class string) string {
	return strings.ToUpper(strings.ReplaceAll(class, "-", "_"))
}
