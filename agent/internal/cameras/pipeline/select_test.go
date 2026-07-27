package pipeline

import "testing"

func TestPick_SyntheticExplicit(t *testing.T) {
	got, err := Pick(Spec{Name: "synthetic"}, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() != "synthetic" {
		t.Fatalf("got %s", got.Name())
	}
}

func TestPick_UnknownNameError(t *testing.T) {
	if _, err := Pick(Spec{Name: "bogus"}, DefaultConfig()); err == nil {
		t.Fatal("expected error")
	}
}

func TestPick_AutoDetect_OnNonLinuxFallsBackToMacOrSynthetic(t *testing.T) {
	// Auto-detect path. On darwin/!linux+arm64 this must return Mac or Synthetic, never Pi5.
	got, err := Pick(Spec{Name: ""}, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() == "pi5" {
		t.Fatal("auto-detect must not select pi5 outside linux/arm64")
	}
}
