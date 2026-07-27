package modules

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/waypointos/waypoint/proxy/internal/db"
)

// CheckUpdater ingests new GitHub releases for a module and bumps auto-update
// rovers. Push publishes the desired-state set for a rover (DesiredPusher.Push).
type CheckUpdater struct {
	Repo     *db.ModulesRepo
	Blobs    BlobStore
	GitHub   *GitHub
	Verifier verifier
	Push     func(ctx context.Context, roverID string) error

	assetBase string // test seam: prefixed onto non-absolute asset URLs
}

type CheckResult struct {
	Ingested          []string `json:"ingested"`
	Latest            string   `json:"latest"`
	AutoUpdatedRovers []string `json:"auto_updated_rovers"`
}

func (c *CheckUpdater) asset(url string) string {
	if c.assetBase != "" && !strings.HasPrefix(url, "http") {
		return c.assetBase + "/" + strings.TrimPrefix(url, "/")
	}
	return url
}

func (c *CheckUpdater) CheckModule(ctx context.Context, moduleID string) (*CheckResult, error) {
	mod, err := c.Repo.GetModule(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	token := ""
	if dec, ok := tokenFromCtx(ctx); ok {
		token = dec
	}
	owner, repo, err := ParseOwnerRepo(mod.SourceRepoURL)
	if err != nil {
		return nil, err
	}

	existingVers, _ := c.Repo.ListVersions(ctx, moduleID)
	known := map[string]bool{}
	var knownList []string
	for _, v := range existingVers {
		known[v.Version] = true
		knownList = append(knownList, v.Version)
	}

	releases, err := c.GitHub.ListReleases(ctx, owner, repo, token)
	if err != nil {
		return nil, err
	}

	res := &CheckResult{Ingested: []string{}, AutoUpdatedRovers: []string{}}
	for _, rel := range releases {
		ver := strings.TrimPrefix(rel.TagName, "v")
		if known[ver] || !IsNewerVersion(ver, knownList) {
			continue
		}
		if err := c.ingestRelease(ctx, mod, ver, rel); err != nil {
			return nil, fmt.Errorf("ingest %s: %w", ver, err)
		}
		known[ver] = true
		knownList = append(knownList, ver)
		res.Ingested = append(res.Ingested, ver)
	}

	// Refresh README (best effort).
	if mdBytes, err := c.GitHub.Readme(ctx, owner, repo, token); err == nil {
		_ = c.Repo.SetReadme(ctx, moduleID, string(mdBytes))
	}

	// Compute latest known version.
	for _, v := range knownList {
		if res.Latest == "" || IsNewerVersion(v, []string{res.Latest}) {
			res.Latest = v
		}
	}

	// Auto-update bumps (only when something new landed).
	if len(res.Ingested) > 0 && res.Latest != "" {
		rovers, err := c.Repo.RoversForModule(ctx, moduleID)
		if err != nil {
			return nil, err
		}
		for _, rm := range rovers {
			if !rm.AutoUpdate || rm.DesiredVersion == res.Latest {
				continue
			}
			if err := c.Repo.SetDesired(ctx, db.SetDesiredInput{
				RoverID: rm.RoverID, ModuleID: moduleID, DesiredVersion: res.Latest, UpdatedBy: mod.RegisteredBy,
			}); err != nil {
				return nil, err
			}
			_ = c.Repo.SetAutoUpdate(ctx, rm.RoverID, moduleID, true)
			if c.Push != nil {
				if err := c.Push(ctx, rm.RoverID); err != nil {
					return nil, err
				}
			}
			res.AutoUpdatedRovers = append(res.AutoUpdatedRovers, rm.RoverID)
		}
	}
	return res, nil
}

func (c *CheckUpdater) ingestRelease(ctx context.Context, mod *db.ModuleRow, ver string, rel GitHubRelease) error {
	token := ""
	if dec, ok := tokenFromCtx(ctx); ok {
		token = dec
	}
	var manURL, rawURL, sigURL string
	for _, a := range rel.Assets {
		switch {
		case a.Name == "manifest.json":
			manURL = c.asset(a.URL)
		case strings.HasSuffix(a.Name, ".raw.cosign"):
			sigURL = c.asset(a.URL)
		case strings.HasSuffix(a.Name, ".raw"):
			rawURL = c.asset(a.URL)
		}
	}
	if manURL == "" || rawURL == "" || sigURL == "" {
		return fmt.Errorf("release %s missing manifest/raw/cosign asset", ver)
	}
	manBytes, err := c.GitHub.Download(ctx, manURL, token)
	if err != nil {
		return err
	}
	var manifest struct {
		Name, Label, Version string
	}
	if err := json.Unmarshal(manBytes, &manifest); err != nil {
		return err
	}
	if manifest.Version != ver {
		return fmt.Errorf("manifest version %q does not match release tag %q", manifest.Version, ver)
	}
	rawBytes, err := c.GitHub.Download(ctx, rawURL, token)
	if err != nil {
		return err
	}
	sigBytes, err := c.GitHub.Download(ctx, sigURL, token)
	if err != nil {
		return err
	}
	if _, err := c.Verifier.Verify(ctx, rawBytes, sigBytes, mod.ExpectedIdentitySAN); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	storedPath, err := c.Blobs.Put(ctx, mod.ModuleID, manifest.Version, bytes.NewReader(rawBytes))
	if err != nil {
		return err
	}
	// Cache the static UI tree like HandleRegister does; without it the
	// module tab 404s on its assets for update-ingested versions.
	if err := cacheStaticTree(ctx, c.Blobs, mod.ModuleID, manifest.Version, rawBytes); err != nil {
		return fmt.Errorf("cache static tree: %w", err)
	}
	sum := sha256.Sum256(rawBytes)
	return c.Repo.IngestVersion(ctx, db.IngestVersionInput{
		ModuleID: mod.ModuleID, Version: manifest.Version, RawPath: storedPath,
		RawSha256: hex.EncodeToString(sum[:]), ManifestJSON: manBytes,
		ReleaseNotesMD: rel.Body, ReleaseHTMLURL: rel.HTMLURL,
	})
}

// tokenCtxKey carries the decrypted GitHub token down to CheckModule without
// widening signatures. The API layer decrypts and attaches it.
type tokenCtxKey struct{}

func withToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenCtxKey{}, token)
}
func tokenFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(tokenCtxKey{}).(string)
	return v, ok
}
