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

// A marketplace deploy carries no config_toml, so it must not clear the config
// an operator entered through the module config form.
func TestAPI_Deploy_PreservesConfigWhenOmitted(t *testing.T) {
	pool := newTestPool(t)
	repo := db.NewModulesRepo(pool)
	u, err := db.NewUsersRepo(pool).UpsertFromWorkOS(context.Background(), "wk_admin_cfg", "admincfg@example.com")
	require.NoError(t, err)
	ctx := authkit.WithUser(context.Background(), &authkit.CurrentUser{ID: u.ID, Email: u.Email, IsAdmin: true})

	_, err = pool.Exec(ctx,
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		"rc1", "rc1", "pubkey-rc1", u.ID)
	require.NoError(t, err)
	require.NoError(t, repo.RegisterModule(ctx, db.RegisterModuleInput{
		ModuleID: "umr", DisplayName: "Connectivity", SourceRepoURL: "https://github.com/waypointos/waypoint-umr",
		ExpectedIdentitySAN: ".*", RegisteredBy: u.ID, RepoVisibility: "public",
	}))
	cfg := "[modules_config.umr]\npassword = \"secret\"\n"
	require.NoError(t, repo.SetDesired(ctx, db.SetDesiredInput{
		RoverID: "rc1", ModuleID: "umr", DesiredVersion: "0.3.0", ConfigTOML: cfg, UpdatedBy: u.ID,
	}))

	api := NewAPI(repo, NewDiskBlobStore(t.TempDir()), http.DefaultClient)
	deploy := func(body []byte) {
		req := httptest.NewRequest("POST", "/api/admin/modules/umr/deploy", bytes.NewReader(body)).WithContext(ctx)
		req.SetPathValue("moduleID", "umr")
		rec := httptest.NewRecorder()
		api.HandleDeploy(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	desiredFor := func() db.DesiredRow {
		rows, err := repo.DesiredForRover(ctx, "rc1")
		require.NoError(t, err)
		require.Len(t, rows, 1)
		return rows[0]
	}

	deploy([]byte(`{"rovers":[{"rover_id":"rc1","version":"0.4.0","auto_update":true}]}`))
	got := desiredFor()
	require.Equal(t, "0.4.0", got.DesiredVersion)
	require.Equal(t, cfg, got.ConfigTOML, "an omitted config_toml must keep the stored config")

	// An explicit empty string still clears it: that is the form saying "no config".
	deploy([]byte(`{"rovers":[{"rover_id":"rc1","version":"0.4.0","auto_update":true,"config_toml":""}]}`))
	require.Empty(t, desiredFor().ConfigTOML)
}
