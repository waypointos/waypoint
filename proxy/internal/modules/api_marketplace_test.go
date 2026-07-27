package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

func TestAPI_Marketplace_And_Deploy(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	u, err := db.NewUsersRepo(pool).UpsertFromWorkOS(context.Background(), "wk_admin_mkt", "adminmkt@example.com")
	require.NoError(t, err)
	ctx := authkit.WithUser(context.Background(), &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})
	uid := u.ID

	// Seed rovers (rover_module_state.rover_id FKs to rovers).
	for _, id := range []string{"r1", "r2"} {
		_, err := pool.Exec(ctx,
			`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
			id, id, "pubkey-"+id, uid)
		require.NoError(t, err)
	}

	require.NoError(t, repo.RegisterModule(ctx, db.RegisterModuleInput{
		ModuleID: "power", DisplayName: "Power", SourceRepoURL: "https://github.com/acme/power",
		ExpectedIdentitySAN: ".*", RegisteredBy: uid, RepoVisibility: "public",
	}))
	require.NoError(t, repo.IngestVersion(ctx, db.IngestVersionInput{ModuleID: "power", Version: "1.0.0", RawPath: "/x", RawSha256: "s", ManifestJSON: []byte("{}")}))

	api := NewAPI(repo, NewDiskBlobStore(t.TempDir()), http.DefaultClient)

	// Deploy to two rovers.
	body, _ := json.Marshal(deployRequest{Rovers: []deployRover{
		{RoverID: "r1", Version: "1.0.0", AutoUpdate: true},
		{RoverID: "r2", Version: "1.0.0", AutoUpdate: false},
	}})
	req := httptest.NewRequest("POST", "/api/admin/modules/power/deploy", bytes.NewReader(body)).WithContext(ctx)
	req.SetPathValue("moduleID", "power")
	rec := httptest.NewRecorder()
	api.HandleDeploy(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Marketplace lists the module with two rovers and latest version.
	mreq := httptest.NewRequest("GET", "/api/admin/marketplace", nil).WithContext(ctx)
	mrec := httptest.NewRecorder()
	api.HandleMarketplace(mrec, mreq)
	require.Equal(t, http.StatusOK, mrec.Code)
	var out struct {
		Modules []struct {
			ModuleID      string `json:"module_id"`
			LatestVersion string `json:"latest_version"`
			Rovers        []struct {
				RoverID         string `json:"rover_id"`
				DesiredVersion  string `json:"desired_version"`
				AutoUpdate      bool   `json:"auto_update"`
				UpdateAvailable bool   `json:"update_available"`
			} `json:"rovers"`
		} `json:"modules"`
	}
	require.NoError(t, json.Unmarshal(mrec.Body.Bytes(), &out))
	require.Len(t, out.Modules, 1)
	require.Equal(t, "1.0.0", out.Modules[0].LatestVersion)
	require.Len(t, out.Modules[0].Rovers, 2)
}
