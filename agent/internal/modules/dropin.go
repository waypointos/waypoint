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
// the component class and state rate to the module process. "" when the
// module declares no component.
func RenderComponentDropin(c *Component) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("[Service]\nEnvironment=WAYPOINT_MODULE_COMPONENT=%s\nEnvironment=WAYPOINT_MODULE_STATE_RATE_HZ=%s\n",
		c.Class, strconv.FormatFloat(c.StateRateHz, 'f', -1, 64))
}
