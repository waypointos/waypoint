// Package sysinfo publishes the host's SystemTelemetry heartbeat
// (waypoint.<rover-id>.telemetry.system) at 1 Hz. It owns the host-side
// view of the rover process: CPU, temperature, image version. The servo
// bus is core's job; impersonation is sim's job; this package is the
// agent's contribution.
package sysinfo

// Probes is the seam between platform-specific host measurements and the
// cross-platform Publisher. Implementations return (value, true) when a
// measurement is available, (_, false) when the field should be left
// absent in the published SystemTelemetry message. Honest N/A — never
// fabricated zeros.
//
// Concurrency: Probes methods are only called from the Publisher's
// single tick goroutine. Implementations need not be safe for concurrent
// use.
type Probes interface {
	CPUPercent() (float64, bool)
	TempC() (float64, bool)

	// MemoryBytes returns (used, total, ok). used = MemTotal - MemAvailable
	// from /proc/meminfo on Linux. ok=false leaves both fields absent in the
	// published SystemTelemetry — never publish zeros for "no measurement".
	MemoryBytes() (used, total uint64, ok bool)

	// UptimeSeconds returns host wall-clock uptime in seconds. Sources from
	// /proc/uptime on Linux. Not the agent process's runtime.
	UptimeSeconds() (uint64, bool)
}

// ImageInfo is the host's view of the current OS image. Fields default
// to the empty string, which the Publisher renders as absent in the
// SystemTelemetry message (rather than publishing "").
type ImageInfo struct {
	Version   string
	Partition string
}

// DefaultProbes returns a Probes appropriate for the host OS. On Linux,
// reads /proc/stat and /sys/class/thermal/thermal_zone0/temp. On other
// platforms, returns a no-op Probes that reports no measurements (Mac
// dev path; CPU/temp render as N/A in the dashboard).
//
// The actual implementation is supplied by probes_linux.go or
// probes_other.go via build tags.
func DefaultProbes() Probes { return defaultProbes() }
