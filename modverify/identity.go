package modverify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// GitHubOIDCIssuer is the OIDC issuer for GitHub Actions keyless signing.
const GitHubOIDCIssuer = "https://token.actions.githubusercontent.com"

// AnyReleaseSAN matches any github.com repo's .github/workflows/release.yml
// signing a v* tag. Used for first-sight (TOFU) discovery, where the concrete
// signer is learned from the cert and pinned afterward.
const AnyReleaseSAN = `^https://github\.com/[^/]+/[^/]+/\.github/workflows/release\.yml@refs/tags/v.+$`

// SigningIdentity reduces a cosign keyless SAN to its version-independent
// workflow identity by dropping the trailing git ref ("@refs/tags/v1.2.3").
// TOFU pins this so the same release workflow can publish new versions, while
// a different repo or workflow still trips the pin. The SAN carries exactly
// one "@" (the ref separator), so the part before it is the identity.
func SigningIdentity(san string) string {
	id, _, _ := strings.Cut(san, "@")
	return id
}

// ExpectedSANForRepo derives the cosign cert SAN regex for a module's release
// workflow from its github.com repo URL.
func ExpectedSANForRepo(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host != "github.com" {
		return "", fmt.Errorf("identity: repo must be a github.com URL")
	}
	path := strings.Trim(u.Path, "/")
	if strings.Count(path, "/") != 1 {
		return "", fmt.Errorf("identity: expected github.com/<owner>/<repo>")
	}
	q := regexp.QuoteMeta(path)
	return `^https://github\.com/` + q + `/\.github/workflows/release\.yml@refs/tags/v.+$`, nil
}
