package images

import (
	"context"
	"strings"
	"time"

	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/modules"
)

// ReleaseChecker catalogs new image releases from the registered GitHub sources.
// It does not download or cosign-verify the .swu; the rover verifies at apply.
type ReleaseChecker struct {
	Repo   *db.ReleasesRepo
	GitHub *modules.GitHub
	// DecryptToken returns the plaintext GitHub token for a source (or "").
	DecryptToken func(src db.ImageSourceRow) string
}

func (c *ReleaseChecker) token(src db.ImageSourceRow) string {
	if c.DecryptToken == nil {
		return ""
	}
	return c.DecryptToken(src)
}

// CheckAll re-reads releases for every source and ingests new ones. Returns the
// number of newly ingested releases.
func (c *ReleaseChecker) CheckAll(ctx context.Context) (int, error) {
	srcs, err := c.Repo.ListSources(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, src := range srcs {
		n, err := c.checkSource(ctx, src)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (c *ReleaseChecker) checkSource(ctx context.Context, src db.ImageSourceRow) (int, error) {
	owner, repo, err := modules.ParseOwnerRepo(src.RepoURL)
	if err != nil {
		return 0, err
	}
	tok := c.token(src)
	releases, err := c.GitHub.ListReleases(ctx, owner, repo, tok)
	if err != nil {
		return 0, err
	}
	existing, _ := c.Repo.ListReleases(ctx, src.Channel)
	known := map[string]bool{}
	for _, r := range existing {
		known[r.Version] = true
	}

	n := 0
	for _, rel := range releases {
		// tags look like image-v0.6.0
		ver := strings.TrimPrefix(strings.TrimPrefix(rel.TagName, "image-"), "v")
		if ver == "" || known[ver] {
			continue
		}
		swuName := "waypoint-" + src.Channel + "-" + ver + ".swu"
		var swuURL, sumsURL string
		for _, a := range rel.Assets {
			switch {
			case a.Name == swuName:
				swuURL = a.BrowserDownloadURL
			case a.Name == "SHA256SUMS":
				sumsURL = a.URL
			}
		}
		if swuURL == "" {
			continue // no asset for this channel in this release
		}
		sha := ""
		if sumsURL != "" {
			if data, derr := c.GitHub.Download(ctx, sumsURL, tok); derr == nil {
				sha = ParseSha256Sums(data)[swuName]
			}
		}
		var publishedAt *time.Time
		if t, perr := time.Parse(time.RFC3339, rel.PublishedAt); perr == nil {
			publishedAt = &t
		}
		if err := c.Repo.IngestRelease(ctx, db.IngestReleaseInput{
			Channel: src.Channel, Version: ver, SwuURL: swuURL, SwuSha256: sha,
			ReleaseNotesMD: rel.Body, ReleaseHTMLURL: rel.HTMLURL, ReleasePublishedAt: publishedAt,
		}); err != nil {
			return n, err
		}
		known[ver] = true
		n++
	}
	return n, nil
}
