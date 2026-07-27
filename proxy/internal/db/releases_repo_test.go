package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleasesRepo_RoundTrip(t *testing.T) {
	pool := newTestPool(t)
	repo := NewReleasesRepo(pool)
	ctx := context.Background()
	u, err := NewUsersRepo(pool).UpsertFromWorkOS(ctx, "wk_rel", "rel@example.com")
	require.NoError(t, err)

	require.NoError(t, repo.RegisterSource(ctx, RegisterSourceInput{
		RepoURL: "https://github.com/acme/waypoint-image", Channel: "prod",
		RepoVisibility: "public", RegisteredBy: u.ID,
	}))
	srcs, err := repo.ListSources(ctx)
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	require.Equal(t, "prod", srcs[0].Channel)
	require.Equal(t, "https://github.com/acme/waypoint-image", srcs[0].RepoURL)

	require.NoError(t, repo.IngestRelease(ctx, IngestReleaseInput{
		Channel: "prod", Version: "0.6.0", SwuURL: "https://gh/dl/x.swu", SwuSha256: "abc",
		ReleaseNotesMD: "## notes", ReleaseHTMLURL: "https://gh/r/0.6.0",
	}))
	require.NoError(t, repo.IngestRelease(ctx, IngestReleaseInput{
		Channel: "prod", Version: "0.5.0", SwuURL: "https://gh/dl/y.swu", SwuSha256: "def",
	}))
	latest, err := repo.LatestRelease(ctx, "prod")
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, "0.6.0", latest.Version)
	require.Equal(t, "## notes", latest.ReleaseNotesMD)

	none, err := repo.LatestRelease(ctx, "dev")
	require.NoError(t, err)
	require.Nil(t, none)
}
