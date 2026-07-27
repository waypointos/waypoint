// Package verify validates a .swu OTA artifact against a Sigstore cosign
// keyless bundle (.cosign).
//
// The signing identity is pinned in code: the only trusted signer is the
// Waypoint image-build GitHub Actions workflow tagged image-v<version>,
// authenticated via GitHub's OIDC issuer. There is no long-lived signing key
// anywhere in the system; trust is rooted in Fulcio (cert authority), Rekor
// (transparency log), and TUF (trusted-root distribution).
//
// Dev images can bypass verification by setting WAYPOINT_SKIP_VERIFY=1. This
// is intentionally an environment variable, not a config flag — production
// images must simply not set it. A loud log line is emitted whenever the
// skip is taken so it cannot pass unnoticed in operator logs.
package verify

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// envSkip, when set to "1", causes Sigstore.Verify to return nil immediately.
// Used for QEMU smoke and dev images. Production images must not set this.
const envSkip = "WAYPOINT_SKIP_VERIFY"

// SkipEnabled reports whether signature verification is disabled via
// WAYPOINT_SKIP_VERIFY=1 (dev/QEMU images). When true, callers should also
// skip requiring a .cosign bundle: dev builds ship none, and Verify is a
// no-op anyway. Production images leave the env unset, so this returns false.
func SkipEnabled() bool { return os.Getenv(envSkip) == "1" }

// Pinned identity for the Waypoint image-build workflow. These values are
// baked into the binary so a compromised config file cannot accept signatures
// from any other source.
const (
	// expectedSANRegex matches the Subject Alternative Name (SAN) URI that
	// Fulcio embeds in a workflow-OIDC certificate. The job ref must be a
	// signed tag of the form image-v<version> on the canonical repo.
	expectedSANRegex = `^https://github\.com/waypointos/waypoint/\.github/workflows/image-build\.yml@refs/tags/image-v.+$`

	// expectedIssuer is GitHub Actions' OIDC issuer URL. This pins the
	// signer's identity provider to GitHub's token service; any other
	// issuer (e.g. a custom OIDC provider used by an attacker) is rejected.
	expectedIssuer = "https://token.actions.githubusercontent.com"
)

// Verifier validates a blob against a bundle. Implementations must enforce
// the pinned identity policy. The interface exists so apply.Handler can be
// unit-tested with a fake verifier.
type Verifier interface {
	Verify(ctx context.Context, blobPath, bundlePath string) error
}

// Sigstore is the production Verifier implementation. It loads the bundle,
// fetches a TUF-rooted trusted root once at construction time, and verifies
// each artifact against pinned identity + a Rekor inclusion proof requirement.
type Sigstore struct {
	once    sync.Once
	initErr error

	// trustedRoot is the cached Sigstore trusted root, fetched lazily via TUF
	// on first Verify call so unit tests that never call Verify do not need
	// network access.
	trustedRoot root.TrustedMaterial

	// verifier is the configured sigstore-go verifier; reused across calls.
	verifier *verify.Verifier

	// policyOpts is the slice of identity policies (pinned cert SAN regex +
	// issuer). Built once and reused.
	policyOpts []verify.PolicyOption
}

// NewSigstore constructs a Sigstore Verifier. The TUF trusted root is fetched
// lazily on the first Verify call so that simple binary-startup paths are not
// blocked on network I/O at boot.
func NewSigstore() *Sigstore {
	return &Sigstore{}
}

// init performs one-shot lazy initialization: fetch the TUF trusted root,
// build the sigstore-go Verifier, and compile the pinned identity policy.
// Called from Verify under a sync.Once.
func (s *Sigstore) init() {
	opts := tuf.DefaultOptions()
	tr, err := root.NewLiveTrustedRoot(opts)
	if err != nil {
		s.initErr = fmt.Errorf("trusted root: %w", err)
		return
	}
	s.trustedRoot = tr

	// Require: (a) a Rekor transparency log inclusion proof, (b) observer
	// timestamps from the log or RFC3161 TSA, and (c) the Fulcio cert to
	// carry a SignedCertificateTimestamp. These three together give us
	// "this cert was issued by Fulcio, logged in Rekor, and signed within
	// its validity window" — the standard cosign keyless guarantee.
	v, err := verify.NewVerifier(
		s.trustedRoot,
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
		verify.WithSignedCertificateTimestamps(1),
	)
	if err != nil {
		s.initErr = fmt.Errorf("new verifier: %w", err)
		return
	}
	s.verifier = v

	certID, err := verify.NewShortCertificateIdentity(
		expectedIssuer, "", // issuer pinned by exact value
		"", expectedSANRegex, // SAN pinned by regex
	)
	if err != nil {
		s.initErr = fmt.Errorf("certificate identity: %w", err)
		return
	}
	s.policyOpts = []verify.PolicyOption{verify.WithCertificateIdentity(certID)}
}

// Verify checks blobPath against bundlePath. It returns nil iff the bundle's
// cert chains to Fulcio, the SAN matches the pinned image-build workflow
// regex, the issuer is GitHub Actions, a Rekor inclusion proof is present
// and verified, and the artifact hash matches the signed digest.
//
// If WAYPOINT_SKIP_VERIFY=1 is set, Verify logs loudly and returns nil
// without touching either file. This is for dev/QEMU only.
func (s *Sigstore) Verify(ctx context.Context, blobPath, bundlePath string) error {
	if SkipEnabled() {
		slog.Warn("VERIFY SKIPPED: WAYPOINT_SKIP_VERIFY=1 set; dev mode")
		return nil
	}

	s.once.Do(s.init)
	if s.initErr != nil {
		return s.initErr
	}

	b, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return fmt.Errorf("load bundle: %w", err)
	}

	blob, err := os.Open(blobPath)
	if err != nil {
		return fmt.Errorf("open blob: %w", err)
	}
	defer blob.Close()

	policy := verify.NewPolicy(verify.WithArtifact(blob), s.policyOpts...)
	if _, err := s.verifier.Verify(b, policy); err != nil {
		return fmt.Errorf("sigstore verify: %w", err)
	}
	return nil
}
