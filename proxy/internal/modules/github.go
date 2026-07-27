package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GitHub is a minimal GitHub REST client for module release ingest. APIBase is
// overridable in tests; production uses https://api.github.com.
type GitHub struct {
	HTTP    *http.Client
	APIBase string
}

func NewGitHub(c *http.Client) *GitHub {
	if c == nil {
		c = http.DefaultClient
	}
	return &GitHub{HTTP: c, APIBase: "https://api.github.com"}
}

type GitHubAsset struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// ParseOwnerRepo extracts owner and repo from an https://github.com/<o>/<r> URL.
func ParseOwnerRepo(repoURL string) (string, string, error) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host != "github.com" {
		return "", "", fmt.Errorf("github: repo must be an https://github.com URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("github: expected github.com/<owner>/<repo>")
	}
	return parts[0], parts[1], nil
}

func (g *GitHub) get(ctx context.Context, url, accept, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	return g.HTTP.Do(req)
}

func (g *GitHub) ListReleases(ctx context.Context, owner, repo, token string) ([]GitHubRelease, error) {
	resp, err := g.get(ctx, fmt.Sprintf("%s/repos/%s/%s/releases", g.APIBase, owner, repo),
		"application/vnd.github+json", token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: list releases HTTP %d", resp.StatusCode)
	}
	var out []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Download fetches a release asset by its API URL. Go strips the Authorization
// header on the cross-host redirect to the signed blob, so private assets work.
func (g *GitHub) Download(ctx context.Context, assetURL, token string) ([]byte, error) {
	resp, err := g.get(ctx, assetURL, "application/octet-stream", token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: download HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (g *GitHub) Readme(ctx context.Context, owner, repo, token string) ([]byte, error) {
	resp, err := g.get(ctx, fmt.Sprintf("%s/repos/%s/%s/readme", g.APIBase, owner, repo),
		"application/vnd.github.raw", token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: readme HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
