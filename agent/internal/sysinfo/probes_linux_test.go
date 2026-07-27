//go:build linux

package sysinfo

import "testing"

func TestParseProcStatSample_HappyPath(t *testing.T) {
	// /proc/stat first line format:
	// "cpu  user nice system idle iowait irq softirq steal guest guest_nice"
	// Fields 1-10 (after "cpu"). Idle = field 4 (1-indexed after label),
	// iowait = field 5. We compute totalIdle = idle + iowait.
	sample := []byte("cpu  100 0 50 800 50 0 0 0 0 0\n")
	idle, total, ok := parseProcStatSample(sample)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if idle != 850 { // 800 idle + 50 iowait
		t.Errorf("idle = %d; want 850", idle)
	}
	if total != 1000 { // 100+0+50+800+50+0+0+0+0+0
		t.Errorf("total = %d; want 1000", total)
	}
}

func TestParseProcStatSample_NotEnoughFields(t *testing.T) {
	if _, _, ok := parseProcStatSample([]byte("cpu  100 0\n")); ok {
		t.Error("expected ok=false for short sample")
	}
}

func TestParseProcStatSample_WrongPrefix(t *testing.T) {
	if _, _, ok := parseProcStatSample([]byte("cpu0 100 0 50 800 50 0 0 0 0 0\n")); ok {
		t.Error("expected ok=false for per-core line (we want aggregate)")
	}
}

func TestCpuPercentFromSamples_HappyPath(t *testing.T) {
	// First sample: 850 idle of 1000 total.
	// Second sample: 900 idle of 1100 total.
	// Delta: 50 idle / 100 total -> 50% idle -> 50% busy.
	pct, ok := cpuPercentFromSamples(850, 1000, 900, 1100)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pct != 50.0 {
		t.Errorf("pct = %v; want 50.0", pct)
	}
}

func TestCpuPercentFromSamples_NoDelta(t *testing.T) {
	if _, ok := cpuPercentFromSamples(100, 200, 100, 200); ok {
		t.Error("expected ok=false when totals are identical (div-by-zero guard)")
	}
}

func TestParseThermalZone_HappyPath(t *testing.T) {
	c, ok := parseThermalZone([]byte("47123\n"))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c < 47.0 || c > 47.2 {
		t.Errorf("c = %v; want ~47.123 (millidegrees / 1000)", c)
	}
}

func TestParseThermalZone_Malformed(t *testing.T) {
	if _, ok := parseThermalZone([]byte("not a number")); ok {
		t.Error("expected ok=false for garbage input")
	}
}

func TestParseMemInfo_HappyPath(t *testing.T) {
	sample := []byte("MemTotal:        8123456 kB\nMemFree:         1000000 kB\nMemAvailable:    3000000 kB\nBuffers: 0 kB\n")
	used, total, ok := parseMemInfo(sample)
	if !ok {
		t.Fatal("expected ok=true")
	}
	const kb = 1024
	if total != 8123456*kb {
		t.Errorf("total = %d; want %d", total, 8123456*kb)
	}
	if used != (8123456-3000000)*kb {
		t.Errorf("used = %d; want %d", used, (8123456-3000000)*kb)
	}
}

func TestParseMemInfo_MissingFields(t *testing.T) {
	// MemAvailable missing — modern kernels always have it, but be defensive.
	sample := []byte("MemTotal:        8123456 kB\n")
	if _, _, ok := parseMemInfo(sample); ok {
		t.Error("expected ok=false when MemAvailable is missing")
	}
}

func TestParseUptime_HappyPath(t *testing.T) {
	// /proc/uptime is "<uptime-seconds> <idle-seconds>" as floats.
	s, ok := parseUptime([]byte("12345.67 98765.43\n"))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if s != 12345 {
		t.Errorf("s = %d; want 12345", s)
	}
}

func TestParseUptime_Malformed(t *testing.T) {
	if _, ok := parseUptime([]byte("garbage")); ok {
		t.Error("expected ok=false for garbage")
	}
}
