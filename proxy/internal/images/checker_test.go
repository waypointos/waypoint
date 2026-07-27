package images

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/modules"
)

func TestChecker_IngestsImageReleases(t *testing.T) {
	pool := dbTestPool(t)
	repo := db.NewReleasesRepo(pool)
	ctx := context.Background()
	u, err := db.NewUsersRepo(pool).UpsertFromWorkOS(ctx, "wk_img", "img@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.RegisterSource(ctx, db.RegisterSourceInput{
		RepoURL: "https://github.com/acme/waypoint-image", Channel: "prod", RepoVisibility: "public", RegisteredBy: u.ID,
	}))

	var gh *httptest.Server
	gh = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_ = json.NewEncoder(w).Encode([]modules.GitHubRelease{{
				TagName: "image-v0.6.0", Body: "## image notes", HTMLURL: "https://gh/r/0.6.0", PublishedAt: "2026-05-20T00:00:00Z",
				Assets: []modules.GitHubAsset{
					{Name: "waypoint-prod-0.6.0.swu", BrowserDownloadURL: "https://dl/waypoint-prod-0.6.0.swu"},
					{Name: "SHA256SUMS", URL: gh.URL + "/SUMS", BrowserDownloadURL: "https://dl/SHA256SUMS"},
				},
			}})
		case r.URL.Path == "/SUMS":
			_, _ = w.Write([]byte("deadbeef  waypoint-prod-0.6.0.swu\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()

	chk := &ReleaseChecker{
		Repo:   repo,
		GitHub: &modules.GitHub{HTTP: gh.Client(), APIBase: gh.URL},
	}
	n, err := chk.CheckAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	latest, err := repo.LatestRelease(ctx, "prod")
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, "0.6.0", latest.Version)
	require.Equal(t, "https://dl/waypoint-prod-0.6.0.swu", latest.SwuURL)
	require.Equal(t, "deadbeef", latest.SwuSha256)
	require.Equal(t, "## image notes", latest.ReleaseNotesMD)
}
