package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_DefaultsInterval(t *testing.T) {
	cfg, err := Load(writeTemp(t, `message = "hi"`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Message != "hi" {
		t.Errorf("message = %q, want hi", cfg.Message)
	}
	if cfg.Interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s default", cfg.Interval)
	}
}

func TestLoad_ExplicitInterval(t *testing.T) {
	cfg, err := Load(writeTemp(t, "interval_s = 10"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != 10*time.Second {
		t.Errorf("interval = %v, want 10s", cfg.Interval)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
