package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIdentity_minimal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.toml")
	contents := `
[rover]
id   = "rover-01"
name = "Recon"

[proxy]
url             = "wss://example.com/nats"
operator_pubkey = "OXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

[nats]
user_jwt    = "eyJ-test"
user_seed   = "SU-test"

[local]
bootstrap_admin_token = "tok-test-1234"
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if id.Rover.ID != "rover-01" {
		t.Errorf("rover.id = %q, want rover-01", id.Rover.ID)
	}
	if id.Local.BootstrapAdminToken != "tok-test-1234" {
		t.Errorf("bootstrap token = %q", id.Local.BootstrapAdminToken)
	}
}

func TestLoadIdentity_missingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Fatal("expected error on missing file")
	}
}

func TestLoadIdentity_requiresRoverID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.toml")
	if err := os.WriteFile(path, []byte(`[local]`+"\n"+`bootstrap_admin_token = "x"`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when rover.id is missing")
	}
}

func TestIdentity_ProxySectionRequiresOperatorPubKey(t *testing.T) {
	doc := `
[rover]
id   = "sim-01"
name = "Sim"

[proxy]
url = "wss://x/leaf"
# missing operator_pubkey

[nats]
user_jwt    = "j"
user_seed   = "s"

[local]
bootstrap_admin_token = "t"
`
	if _, err := ParseString(doc); err == nil {
		t.Fatal("expected error on missing operator_pubkey")
	}
}

func TestIdentity_LocalOnlyValid(t *testing.T) {
	doc := `
[rover]
id   = "sim-01"
name = "Sim"

[local]
bootstrap_admin_token = "t"
`
	if _, err := ParseString(doc); err != nil {
		t.Fatalf("local-only should parse: %v", err)
	}
}

func TestIdentity_TwoCameras(t *testing.T) {
	doc := `
[rover]
id   = "sim-01"
name = "Sim"

[local]
bootstrap_admin_token = "t"

[[cameras]]
name     = "chassis-front"
device   = "/dev/video0"
pipeline = "pi5"

[[cameras]]
name     = "gripper"
device   = "/dev/video2"
pipeline = "pi5"
`
	id, err := ParseString(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Cameras) != 2 {
		t.Fatalf("got %d cameras, want 2", len(id.Cameras))
	}
	if id.Cameras[1].Name != "gripper" {
		t.Fatalf("got %q, want gripper", id.Cameras[1].Name)
	}
}

func TestIdentity_DuplicateCameraName(t *testing.T) {
	doc := `
[rover]
id   = "sim-01"
name = "Sim"

[local]
bootstrap_admin_token = "t"

[[cameras]]
name = "a"

[[cameras]]
name = "a"
`
	if _, err := ParseString(doc); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestIdentity_CameraMissingName(t *testing.T) {
	doc := `
[rover]
id   = "sim-01"
name = "Sim"

[local]
bootstrap_admin_token = "t"

[[cameras]]
device = "/dev/video0"
`
	if _, err := ParseString(doc); err == nil {
		t.Fatal("expected missing-name error")
	}
}
