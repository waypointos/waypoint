package modverify

import (
	"bytes"
	"context"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	sgverify "github.com/sigstore/sigstore-go/pkg/verify"
)

// Verifier checks a .raw against its cosign bundle and returns the signer SAN.
type Verifier struct {
	v *sgverify.Verifier
}

// New builds a Verifier from caller-supplied trusted material.
func New(trust root.TrustedMaterial) (*Verifier, error) {
	// require exactly one of each evidence type (tlog entry, observed timestamp, SCT).
	v, err := sgverify.NewVerifier(trust,
		sgverify.WithTransparencyLog(1),
		sgverify.WithObserverTimestamps(1),
		sgverify.WithSignedCertificateTimestamps(1),
	)
	if err != nil {
		return nil, fmt.Errorf("new verifier: %w", err)
	}
	return &Verifier{v: v}, nil
}

// NewFromTUF builds a Verifier from live Sigstore TUF (requires outbound network at init).
func NewFromTUF() (*Verifier, error) {
	tr, err := root.NewLiveTrustedRoot(tuf.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("trusted root (tuf): %w", err)
	}
	return New(tr)
}

// NewFromFile builds a Verifier from a baked trusted_root.json (no network; offline/air-gapped).
func NewFromFile(path string) (*Verifier, error) {
	tr, err := root.NewTrustedRootFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("trusted root (%s): %w", path, err)
	}
	return New(tr)
}

// Verify checks raw against bundleJSON, requiring the cert SAN (URI) to match
// sanPattern (a regex) issued by GitHub Actions OIDC, and returns the concrete
// signer SAN read from the bundle's certificate. No network at verify time.
func (vf *Verifier) Verify(ctx context.Context, raw, bundleJSON []byte, sanPattern string) (string, error) {
	certID, err := sgverify.NewShortCertificateIdentity(GitHubOIDCIssuer, "", "", sanPattern)
	if err != nil {
		return "", fmt.Errorf("certificate identity: %w", err)
	}
	var b bundle.Bundle
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return "", fmt.Errorf("load bundle: %w", err)
	}
	policy := sgverify.NewPolicy(sgverify.WithArtifact(bytes.NewReader(raw)),
		sgverify.WithCertificateIdentity(certID))
	if _, err := vf.v.Verify(&b, policy); err != nil {
		return "", fmt.Errorf("cosign verify: %w", err)
	}
	vc, err := b.VerificationContent()
	if err != nil {
		return "", fmt.Errorf("verification content: %w", err)
	}
	cert := vc.Certificate()
	if cert == nil || len(cert.URIs) == 0 {
		return "", fmt.Errorf("no URI SAN on signing certificate")
	}
	// cosign keyless certs carry exactly one URI SAN (the signer identity).
	return cert.URIs[0].String(), nil
}
