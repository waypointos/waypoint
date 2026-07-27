package images

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

func TestAPI_Fleet(t *testing.T) {
	pool := dbTestPool(t)
	relRepo := db.NewReleasesRepo(pool)
	roversRepo := db.NewRoversRepo(pool)
	ctx := context.Background()
	u, err := db.NewUsersRepo(pool).UpsertFromWorkOS(ctx, "wk_imgapi", "imgapi@example.com")
	require.NoError(t, err)
	adminCtx := authkit.WithUser(ctx, &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})

	_, err = pool.Exec(ctx, `INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1,$2,$3,$4)`,
		"rover-01", "Rover One", "pk", u.ID)
	require.NoError(t, err)
	require.NoError(t, roversRepo.UpdateImageVersion(ctx, "rover-01", "0.5.0"))
	require.NoError(t, relRepo.RegisterSource(ctx, db.RegisterSourceInput{
		RepoURL: "https://github.com/acme/waypoint-image", Channel: "prod", RepoVisibility: "public", RegisteredBy: u.ID,
	}))
	require.NoError(t, relRepo.IngestRelease(ctx, db.IngestReleaseInput{
		Channel: "prod", Version: "0.6.0", SwuURL: "https://dl/x.swu", SwuSha256: "abc", ReleaseNotesMD: "## notes",
	}))

	api := NewAPI(relRepo, roversRepo, nil, nil)

	req := httptest.NewRequest("GET", "/api/admin/releases", nil).WithContext(adminCtx)
	rec := httptest.NewRecorder()
	api.HandleReleasesFleet(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		Rovers []struct {
			RoverID         string `json:"rover_id"`
			CurrentVersion  string `json:"current_version"`
			Channel         string `json:"channel"`
			LatestVersion   string `json:"latest_version"`
			UpdateAvailable bool   `json:"update_available"`
			SwuURL          string `json:"swu_url"`
			SwuSha256       string `json:"swu_sha256"`
			ReleaseNotesMD  string `json:"release_notes_md"`
		} `json:"rovers"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Rovers, 1)
	require.Equal(t, "rover-01", out.Rovers[0].RoverID)
	require.Equal(t, "0.5.0", out.Rovers[0].CurrentVersion)
	require.Equal(t, "prod", out.Rovers[0].Channel)
	require.Equal(t, "0.6.0", out.Rovers[0].LatestVersion)
	require.True(t, out.Rovers[0].UpdateAvailable)
	require.Equal(t, "https://dl/x.swu", out.Rovers[0].SwuURL)
	require.Equal(t, "## notes", out.Rovers[0].ReleaseNotesMD)
}
