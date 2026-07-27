package image

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/waypointos/waypoint/agent/internal/partition"
)

func TestLoad_MissingImageTOMLReturnsSynthetic(t *testing.T) {
	dir := t.TempDir()
	l := &Loader{
		ImageTOMLPath: filepath.Join(dir, "image.toml"),
		BootcountPath: filepath.Join(dir, "bootcount"),
	}
	s, err := l.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if s.Version != "0.0.0-dev" || s.Variant != "dev" || s.Partition != "A" {
		t.Fatalf("synthetic state mismatch: %+v", s)
	}
	if s.Bootcount != 0 || s.BuiltAt != "" {
		t.Fatalf("synthetic state non-zero defaults: %+v", s)
	}
}

func TestLoad_WithBootcount(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "image.toml")
	bootPath := filepath.Join(dir, "bootcount")

	contents := `[image]
version = "0.5.1"
variant = "prod"
partition = "B"
built_at = "2026-05-12T00:00:00Z"
`
	if err := os.WriteFile(tomlPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootPath, []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := &Loader{ImageTOMLPath: tomlPath, BootcountPath: bootPath}
	s, err := l.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want := State{
		Version:   "0.5.1",
		Variant:   "prod",
		Partition: "B",
		BuiltAt:   "2026-05-12T00:00:00Z",
		Bootcount: 2,
	}
	if *s != want {
		t.Fatalf("Load() = %+v, want %+v", *s, want)
	}
}

// Partition must come from the running kernel cmdline, not the static
// image.toml field: the baked value is "A" in both slots, so a rover booted
// from B must still report "B".
func TestLoad_PartitionFromCmdlineOverridesTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "image.toml")
	cmdlinePath := filepath.Join(dir, "cmdline")

	contents := `[image]
version = "0.5.0-dev"
variant = "dev"
partition = "A"
built_at = "2026-06-18T15:27:42Z"
`
	if err := os.WriteFile(tomlPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cmdlinePath, []byte("root=PARTUUID="+partition.BUUID+" ro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := &Loader{ImageTOMLPath: tomlPath, CmdlinePath: cmdlinePath, BootcountPath: filepath.Join(dir, "bootcount")}
	s, err := l.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if s.Partition != "B" {
		t.Fatalf("Partition = %q, want B (runtime cmdline must win over image.toml)", s.Partition)
	}
}

func TestSignalHealthy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "healthy")
	if err := SignalHealthy(path); err != nil {
		t.Fatalf("SignalHealthy: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("healthy marker missing: %v", err)
	}
}
