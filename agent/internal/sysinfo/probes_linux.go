//go:build linux

package sysinfo

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

const (
	procStatPath    = "/proc/stat"
	thermalZonePath = "/sys/class/thermal/thermal_zone0/temp"
	procMemInfoPath = "/proc/meminfo"
	procUptimePath  = "/proc/uptime"
)

// linuxProbes is stateful for CPU% — the metric is meaningful only as a
// delta between two /proc/stat reads. The first call returns
// (_, false); subsequent calls return the percentage between the
// previous and current samples.
//
// Concurrency: not safe. The Publisher calls Probes from one goroutine.
type linuxProbes struct {
	prevIdle, prevTotal uint64
	hasPrev             bool
}

func defaultProbes() Probes { return &linuxProbes{} }

func (p *linuxProbes) CPUPercent() (float64, bool) {
	data, err := os.ReadFile(procStatPath)
	if err != nil {
		return 0, false
	}
	// First line only; that's the aggregate "cpu" entry.
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data = data[:i+1]
	}
	idle, total, ok := parseProcStatSample(data)
	if !ok {
		return 0, false
	}
	if !p.hasPrev {
		p.prevIdle, p.prevTotal, p.hasPrev = idle, total, true
		return 0, false // first call: no delta available
	}
	pct, ok := cpuPercentFromSamples(p.prevIdle, p.prevTotal, idle, total)
	p.prevIdle, p.prevTotal = idle, total
	return pct, ok
}

func (p *linuxProbes) TempC() (float64, bool) {
	data, err := os.ReadFile(thermalZonePath)
	if err != nil {
		return 0, false
	}
	return parseThermalZone(data)
}

func (p *linuxProbes) MemoryBytes() (used, total uint64, ok bool) {
	data, err := os.ReadFile(procMemInfoPath)
	if err != nil {
		return 0, 0, false
	}
	return parseMemInfo(data)
}

func (p *linuxProbes) UptimeSeconds() (uint64, bool) {
	data, err := os.ReadFile(procUptimePath)
	if err != nil {
		return 0, false
	}
	return parseUptime(data)
}

// parseProcStatSample reads the aggregate "cpu" line of /proc/stat
// (user nice system idle iowait irq softirq steal guest guest_nice) and
// returns (totalIdle, totalAll, ok). totalIdle = idle + iowait.
func parseProcStatSample(data []byte) (idle, total uint64, ok bool) {
	line := strings.TrimRight(string(data), "\n")
	fields := strings.Fields(line)
	// "cpu" + at least 10 numeric fields. Reject "cpu0", "cpu1", etc. —
	// we only want the aggregate.
	if len(fields) < 11 || fields[0] != "cpu" {
		return 0, 0, false
	}
	values := make([]uint64, 0, 10)
	for _, f := range fields[1:11] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		values = append(values, v)
	}
	idle = values[3] + values[4] // idle + iowait
	for _, v := range values {
		total += v
	}
	return idle, total, true
}

// cpuPercentFromSamples returns the percent of time NOT spent idle
// between two /proc/stat samples. Returns (0, false) when totals are
// identical (no time passed -> no meaningful percent).
func cpuPercentFromSamples(prevIdle, prevTotal, idle, total uint64) (float64, bool) {
	dTotal := total - prevTotal
	if dTotal == 0 {
		return 0, false
	}
	dIdle := idle - prevIdle
	busyFrac := 1.0 - float64(dIdle)/float64(dTotal)
	return busyFrac * 100.0, true
}

// parseThermalZone reads the millidegree-Celsius integer written to a
// /sys/class/thermal/thermal_zone*/temp file and returns degrees C.
func parseThermalZone(data []byte) (float64, bool) {
	s := strings.TrimSpace(string(data))
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return float64(n) / 1000.0, true
}

// parseMemInfo extracts (used, total) bytes from /proc/meminfo, using
// MemTotal and MemAvailable. MemAvailable is the modern kernel's estimate of
// memory that can be allocated without swapping — closer to operator
// intuition than (MemTotal - MemFree), which excludes reclaimable caches.
func parseMemInfo(data []byte) (used, total uint64, ok bool) {
	var memTotalKb, memAvailKb uint64
	var sawTotal, sawAvail bool
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotalKb, sawTotal = parseMemInfoKb(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailKb, sawAvail = parseMemInfoKb(line)
		}
		if sawTotal && sawAvail {
			break
		}
	}
	if !sawTotal || !sawAvail || memAvailKb > memTotalKb {
		return 0, 0, false
	}
	const kb = 1024
	return (memTotalKb - memAvailKb) * kb, memTotalKb * kb, true
}

func parseMemInfoKb(line string) (uint64, bool) {
	// "MemTotal:        8123456 kB"
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	n, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseUptime reads /proc/uptime which is "<seconds-up> <seconds-idle>"
// as floats. We only need the first field, and we return whole seconds.
func parseUptime(data []byte) (uint64, bool) {
	s := strings.TrimSpace(string(data))
	first := strings.Fields(s)
	if len(first) == 0 {
		return 0, false
	}
	f, err := strconv.ParseFloat(first[0], 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return uint64(f), true
}
