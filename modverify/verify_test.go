package modverify

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpectedSANForRepo_Valid(t *testing.T) {
	san, err := ExpectedSANForRepo("https://github.com/waypointos/waypoint-powermonitor")
	require.NoError(t, err)
	require.Regexp(t, san, "https://github.com/waypointos/waypoint-powermonitor/.github/workflows/release.yml@refs/tags/v0.1.0")
	require.NotRegexp(t, san, "https://github.com/evil/repo/.github/workflows/release.yml@refs/tags/v0.1.0")
}

func TestSigningIdentity(t *testing.T) {
	const base = "https://github.com/o/r/.github/workflows/release.yml"
	// Two tags of the same workflow reduce to the same identity.
	require.Equal(t, base, SigningIdentity(base+"@refs/tags/v0.2.0"))
	require.Equal(t, base, SigningIdentity(base+"@refs/tags/v0.2.1"))
	// A SAN without a ref is returned unchanged.
	require.Equal(t, base, SigningIdentity(base))
	// A different repo yields a different identity.
	require.NotEqual(t, SigningIdentity(base+"@refs/tags/v1"),
		SigningIdentity("https://github.com/evil/r/.github/workflows/release.yml@refs/tags/v1"))
}

func TestExpectedSANForRepo_RejectsNonGitHub(t *testing.T) {
	_, err := ExpectedSANForRepo("https://gitlab.com/x/y")
	require.Error(t, err)
}

func TestExpectedSANForRepo_RejectsBareHost(t *testing.T) {
	_, err := ExpectedSANForRepo("https://github.com/singlepath")
	require.Error(t, err)
}

// Offline-verification fixtures: a real keyless-signed artifact from the umr
// module's GitHub release (v0.2.0, signed with --new-bundle-format) plus the
// pinned public-good Sigstore trusted root. This exercises the exact path the
// agent runs on-rover (modverify.NewFromFile + Verify), with no network.
const (
	fixtureRaw    = "testdata/umr-0.2.0.raw"
	fixtureBundle = "testdata/umr-0.2.0.raw.cosign"
	fixtureRoot   = "testdata/trusted_root.json"
)

func loadFixture(t *testing.T) (*Verifier, []byte, []byte) {
	t.Helper()
	raw, err := os.ReadFile(fixtureRaw)
	require.NoError(t, err)
	bundle, err := os.ReadFile(fixtureBundle)
	require.NoError(t, err)
	v, err := NewFromFile(fixtureRoot)
	require.NoError(t, err)
	return v, raw, bundle
}

func TestVerify_OfflineValid(t *testing.T) {
	v, raw, bundle := loadFixture(t)
	san, err := v.Verify(context.Background(), raw, bundle, AnyReleaseSAN)
	require.NoError(t, err)
	require.Contains(t, san, "/.github/workflows/release.yml@refs/tags/v")
	require.Contains(t, san, "waypoint-ubiquiti-mobile-router")
}

func TestVerify_TamperedArtifact(t *testing.T) {
	v, _, bundle := loadFixture(t)
	_, err := v.Verify(context.Background(), []byte("not the signed bytes"), bundle, AnyReleaseSAN)
	require.Error(t, err)
}

func TestVerify_WrongSANPattern(t *testing.T) {
	v, raw, bundle := loadFixture(t)
	_, err := v.Verify(context.Background(), raw, bundle,
		`^https://github\.com/someone/else/\.github/workflows/release\.yml@refs/tags/v.+$`)
	require.Error(t, err)
}
