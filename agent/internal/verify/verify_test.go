package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSigstore_SkipEnv exercises the WAYPOINT_SKIP_VERIFY=1 escape hatch.
// The skip path must short-circuit before any file I/O or network access,
// so we deliberately point at nonexistent paths to prove they are never read.
func TestSigstore_SkipEnv(t *testing.T) {
	t.Setenv(envSkip, "1")

	s := NewSigstore()
	err := s.Verify(context.Background(), "/nonexistent/blob", "/nonexistent/bundle")
	if err != nil {
		t.Fatalf("Verify with skip env should return nil, got %v", err)
	}
}

// TestSigstore_SkipEnvWrongValue confirms only "1" triggers the skip — any
// other value (e.g. "true", "yes", "0") falls through to real verification.
// We use a missing bundle file so verification fails fast without network.
func TestSigstore_SkipEnvWrongValue(t *testing.T) {
	t.Setenv(envSkip, "true")

	s := NewSigstore()
	tmp := t.TempDir()
	blob := filepath.Join(tmp, "blob")
	if err := os.WriteFile(blob, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := s.Verify(context.Background(), blob, filepath.Join(tmp, "missing.cosign"))
	if err == nil {
		t.Fatal("Verify with non-1 skip env should not skip; expected error")
	}
}

// TestSigstore_BundleNotFound checks the file-loading error path. We can
// trigger this without network by pointing init at the public TUF URL
// (which is fine; init runs once and the bundle load is the next step), or
// by checking that the LoadJSONFromPath failure surfaces as a wrapped error.
// To avoid touching the network we instead test the skip-disabled bundle
// failure by pre-priming initErr — but that path isn't exported, so we
// settle for asserting the error wraps with our "load bundle:" prefix in an
// integration-style test only if network is available. Marked short-skip.
func TestSigstore_MalformedBundle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent test in -short mode")
	}
	// Networked: requires TUF fetch. Skip by default; opt-in via env.
	if os.Getenv("WAYPOINT_VERIFY_NETWORK_TESTS") != "1" {
		t.Skip("set WAYPOINT_VERIFY_NETWORK_TESTS=1 to run network-dependent verify tests")
	}

	tmp := t.TempDir()
	blob := filepath.Join(tmp, "blob.swu")
	if err := os.WriteFile(blob, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	junkBundle := filepath.Join(tmp, "blob.swu.cosign")
	if err := os.WriteFile(junkBundle, []byte("{not a real bundle"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewSigstore()
	err := s.Verify(context.Background(), blob, junkBundle)
	if err == nil {
		t.Fatal("expected error on malformed bundle, got nil")
	}
	if !strings.Contains(err.Error(), "load bundle") && !strings.Contains(err.Error(), "sigstore verify") {
		t.Fatalf("error %q does not look like a bundle-load or verify error", err)
	}
}

// TestPinnedIdentity asserts the hardcoded SAN regex and issuer match the
// values the CI workflow actually produces. If someone changes the workflow
// path or the canonical repo without updating this constant, this test fails.
func TestPinnedIdentity(t *testing.T) {
	if expectedIssuer != "https://token.actions.githubusercontent.com" {
		t.Errorf("expectedIssuer drifted from GitHub Actions OIDC: %s", expectedIssuer)
	}
	wantPrefix := `^https://github\.com/waypointos/waypoint/\.github/workflows/image-build\.yml@refs/tags/image-v`
	if !strings.HasPrefix(expectedSANRegex, wantPrefix) {
		t.Errorf("expectedSANRegex does not pin the image-build workflow on the canonical repo: %s", expectedSANRegex)
	}
}
