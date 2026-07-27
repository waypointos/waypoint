//go:build linux

package modules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnitName_ConcreteHyphenForm(t *testing.T) {
	if got := unitName("power-monitor"); got != "waypoint-module-power-monitor.service" {
		t.Fatalf("unitName = %q, want waypoint-module-power-monitor.service", got)
	}
}

// TestSystemdSupervisor_StartStop uses a fake systemctl on PATH.
func TestSystemdSupervisor_StartStop(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "systemctl")
	// Echo args to a log file; exit 0.
	script := "#!/bin/sh\necho \"$@\" >> " + dir + "/calls.log\nexit 0\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	sup := NewSystemdSupervisor()
	if err := sup.Start(context.Background(), "umr"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := sup.Stop(context.Background(), "umr"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "calls.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := "start waypoint-module-umr.service\nstop waypoint-module-umr.service\n"
	if string(got) != want {
		t.Fatalf("calls:\n got:%s\nwant:%s", got, want)
	}
}
