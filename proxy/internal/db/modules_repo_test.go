package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModulesRepo_RegisterModule(t *testing.T) {
	pool := newTestPool(t)
	repo := NewModulesRepo(pool)
	usersRepo := NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin", "admin@example.com")
	require.NoError(t, err)

	require.NoError(t, repo.RegisterModule(context.Background(), RegisterModuleInput{
		ModuleID:            "power-monitor",
		DisplayName:         "Power Monitor",
		SourceRepoURL:       "https://github.com/x/waypoint-module-power-monitor",
		ExpectedIdentitySAN: "^https://github.com/x/y/.github/workflows/release.yml@refs/tags/v.+$",
		RegisteredBy:        u.ID,
	}))
	got, err := repo.GetModule(context.Background(), "power-monitor")
	require.NoError(t, err)
	require.Equal(t, "Power Monitor", got.DisplayName)
}

func TestModulesRepo_IngestVersion(t *testing.T) {
	pool := newTestPool(t)
	repo := NewModulesRepo(pool)
	usersRepo := NewUsersRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin", "admin@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.RegisterModule(context.Background(), RegisterModuleInput{
		ModuleID: "x", DisplayName: "X", SourceRepoURL: "https://example/x",
		ExpectedIdentitySAN: "^https://github.com/x/y/.github/workflows/release.yml@refs/tags/v.+$",
		RegisteredBy: u.ID,
	}))
	require.NoError(t, repo.IngestVersion(context.Background(), IngestVersionInput{
		ModuleID: "x", Version: "0.1.0",
		RawPath: "/var/lib/waypoint-proxy/modules/x-0.1.0.raw",
		RawSha256: "abc", ManifestJSON: []byte(`{"name":"x"}`),
	}))
	versions, err := repo.ListVersions(context.Background(), "x")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "0.1.0", versions[0].Version)
}

func TestModulesRepo_SetDesired(t *testing.T) {
	pool := newTestPool(t)
	repo := NewModulesRepo(pool)
	usersRepo := NewUsersRepo(pool)
	roversRepo := NewRoversRepo(pool)
	u, err := usersRepo.UpsertFromWorkOS(context.Background(), "wk_admin", "admin@example.com")
	require.NoError(t, err)
	// Seed a rover row directly (the repo's enroll path is more involved).
	_, err = pool.Exec(context.Background(),
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		"rover-01", "Test Rover", "pubkey-placeholder", u.ID)
	require.NoError(t, err)
	_ = roversRepo

	require.NoError(t, repo.RegisterModule(context.Background(), RegisterModuleInput{
		ModuleID: "x", DisplayName: "X", SourceRepoURL: "https://example/x",
		ExpectedIdentitySAN: "^https://github.com/x/y/.github/workflows/release.yml@refs/tags/v.+$",
		RegisteredBy: u.ID,
	}))
	require.NoError(t, repo.SetDesired(context.Background(), SetDesiredInput{
		RoverID: "rover-01", ModuleID: "x", DesiredVersion: "0.1.0",
		ConfigTOML: "[modules_config.x]\nhost = \"https://test\"\n",
		UpdatedBy: u.ID,
	}))
	got, err := repo.DesiredForRover(context.Background(), "rover-01")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "0.1.0", got[0].DesiredVersion)
}

func TestModulesRepo_Marketplace(t *testing.T) {
	pool := newTestPool(t)
	repo := NewModulesRepo(pool)
	ctx := context.Background()
	u, err := NewUsersRepo(pool).UpsertFromWorkOS(ctx, "wk_mkt", "mkt@example.com")
	require.NoError(t, err)
	uid := u.ID

	require.NoError(t, repo.RegisterModule(ctx, RegisterModuleInput{
		ModuleID: "power", DisplayName: "Power", SourceRepoURL: "https://github.com/acme/power",
		ExpectedIdentitySAN: "san", RegisteredBy: uid, RepoVisibility: "private",
		GithubTokenEnc: []byte{1, 2, 3},
	}))

	row, err := repo.GetModule(ctx, "power")
	require.NoError(t, err)
	require.Equal(t, "private", row.RepoVisibility)
	require.Equal(t, []byte{1, 2, 3}, row.GithubTokenEnc)

	require.NoError(t, repo.IngestVersion(ctx, IngestVersionInput{
		ModuleID: "power", Version: "1.0.0", RawPath: "/p", RawSha256: "sha",
		ManifestJSON: []byte("{}"), ReleaseNotesMD: "## notes", ReleaseHTMLURL: "https://gh/r",
	}))
	vers, err := repo.ListVersions(ctx, "power")
	require.NoError(t, err)
	require.Equal(t, "## notes", vers[0].ReleaseNotesMD)
	require.Equal(t, "https://gh/r", vers[0].ReleaseHTMLURL)

	require.NoError(t, repo.SetReadme(ctx, "power", "# Power"))
	md, err := repo.GetReadme(ctx, "power")
	require.NoError(t, err)
	require.Equal(t, "# Power", md)

	_, err = pool.Exec(ctx,
		`INSERT INTO rovers (id, name, account_pubkey, enrolled_by_user_id) VALUES ($1, $2, $3, $4)`,
		"r1", "Rover 1", "pubkey-r1", uid)
	require.NoError(t, err)

	require.NoError(t, repo.SetDesired(ctx, SetDesiredInput{
		RoverID: "r1", ModuleID: "power", DesiredVersion: "1.0.0", UpdatedBy: uid,
	}))
	require.NoError(t, repo.SetAutoUpdate(ctx, "r1", "power", true))
	rovers, err := repo.RoversForModule(ctx, "power")
	require.NoError(t, err)
	require.Len(t, rovers, 1)
	require.Equal(t, "r1", rovers[0].RoverID)
	require.Equal(t, "1.0.0", rovers[0].DesiredVersion)
	require.True(t, rovers[0].AutoUpdate)
}
