package modules

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

func TestCheckUpdates_IngestsNewAndAutoUpdates(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	ctx := context.Background()

	usersRepo := db.NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(ctx, "wk", "a@b.c")
	require.NoError(t, err)

	// Seed the two rovers (rover_module_state.rover_id FKs to rovers).
	for _, id := range []string{"auto", "pinned"} {
		_, err = pool.Exec(ctx,
			`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
			id, id, "pubkey-placeholder-"+id, u.ID)
		require.NoError(t, err)
	}

	require.NoError(t, repo.RegisterModule(ctx, db.RegisterModuleInput{
		ModuleID: "power", DisplayName: "Power", SourceRepoURL: "https://github.com/acme/power",
		ExpectedIdentitySAN: ".*", RegisteredBy: u.ID, RepoVisibility: "public",
	}))
	require.NoError(t, repo.IngestVersion(ctx, db.IngestVersionInput{ModuleID: "power", Version: "1.0.0", RawPath: "/x", RawSha256: "s", ManifestJSON: []byte("{}")}))
	require.NoError(t, repo.SetDesired(ctx, db.SetDesiredInput{RoverID: "auto", ModuleID: "power", DesiredVersion: "1.0.0", UpdatedBy: u.ID}))
	require.NoError(t, repo.SetAutoUpdate(ctx, "auto", "power", true))
	require.NoError(t, repo.SetDesired(ctx, db.SetDesiredInput{RoverID: "pinned", ModuleID: "power", DesiredVersion: "1.0.0", UpdatedBy: u.ID}))

	// A real (UI-less) squashfs: ingest opens the image to cache its
	// static tree, so literal fake bytes would fail the superblock read.
	raw, err := os.ReadFile("testdata/noui-0.1.0.raw")
	require.NoError(t, err)
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			json.NewEncoder(w).Encode([]GitHubRelease{{
				TagName: "v1.1.0", Body: "## changes", HTMLURL: "https://gh/r/1.1.0", PublishedAt: "2026-05-20T00:00:00Z",
				Assets: []GitHubAsset{
					{Name: "manifest.json", URL: "MAN"},
					{Name: "power-1.1.0.raw", URL: "RAW"},
					{Name: "power-1.1.0.raw.cosign", URL: "SIG"},
				},
			}})
		case strings.HasSuffix(r.URL.Path, "/readme"):
			w.Write([]byte("# Power"))
		case r.URL.Path == "/MAN":
			w.Write([]byte(`{"name":"power","label":"Power","version":"1.1.0"}`))
		case r.URL.Path == "/RAW":
			w.Write(raw)
		case r.URL.Path == "/SIG":
			w.Write([]byte(`{"fake":"bundle"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()

	var pushed []string
	svc := &CheckUpdater{
		Repo:      repo,
		Blobs:     NewDiskBlobStore(t.TempDir()),
		GitHub:    &GitHub{HTTP: gh.Client(), APIBase: gh.URL},
		Verifier:  fakeVerifier{},
		Push:      func(_ context.Context, roverID string) error { pushed = append(pushed, roverID); return nil },
		assetBase: gh.URL,
	}
	res, err := svc.CheckModule(ctx, "power")
	require.NoError(t, err)
	require.Equal(t, []string{"1.1.0"}, res.Ingested)
	require.Equal(t, "1.1.0", res.Latest)
	require.Equal(t, []string{"auto"}, res.AutoUpdatedRovers)

	rovers, _ := repo.RoversForModule(ctx, "power")
	byID := map[string]string{}
	for _, rm := range rovers {
		byID[rm.RoverID] = rm.DesiredVersion
	}
	require.Equal(t, "1.1.0", byID["auto"])
	require.Equal(t, "1.0.0", byID["pinned"])
	require.Equal(t, []string{"auto"}, pushed)

	md, _ := repo.GetReadme(ctx, "power")
	require.Contains(t, md, "# Power")
}
