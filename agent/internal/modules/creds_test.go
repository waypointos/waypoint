package modules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waypointos/waypoint/agent/internal/localauth"
)

func TestWriteRuntimeFiles(t *testing.T) {
	runtimeDir := t.TempDir()
	spec := ModuleSpec{
		Manifest: Manifest{Name: "umr"},
		Config:   map[string]any{"host": "https://192.168.105.1", "password": "secret"},
	}
	creds := localauth.Credentials{Username: "module-umr", Password: "abcd"}
	paths, err := WriteRuntimeFiles(runtimeDir, spec, creds)
	if err != nil {
		t.Fatalf("WriteRuntimeFiles: %v", err)
	}
	verifyFile(t, paths.ConfigPath, 0o600)
	verifyFile(t, paths.CredsPath, 0o600)

	// Idempotent — second call overwrites cleanly.
	if _, err := WriteRuntimeFiles(runtimeDir, spec, creds); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got, err := os.ReadFile(paths.CredsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "module-umr") || !strings.Contains(string(got), "abcd") {
		t.Fatalf("creds file missing fields: %s", got)
	}
}

func verifyFile(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := st.Mode().Perm(); got != want {
		t.Fatalf("%s perms = %v, want %v", filepath.Base(path), got, want)
	}
}
