package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/mod/semver"
)

type ImageSourceRow struct {
	ID             uuid.UUID
	RepoURL        string
	Channel        string
	RepoVisibility string
	GithubTokenEnc []byte
	RegisteredBy   uuid.UUID
	RegisteredAt   time.Time
}

type ImageReleaseRow struct {
	Channel            string
	Version            string
	SwuURL             string
	SwuSha256          string
	ReleaseNotesMD     string
	ReleasePublishedAt *time.Time
	ReleaseHTMLURL     string
	IngestedAt         time.Time
}

type RegisterSourceInput struct {
	RepoURL, Channel, RepoVisibility string
	GithubTokenEnc                   []byte
	RegisteredBy                     uuid.UUID
}

type IngestReleaseInput struct {
	Channel, Version, SwuURL, SwuSha256 string
	ReleaseNotesMD, ReleaseHTMLURL      string
	ReleasePublishedAt                  *time.Time
}

type ReleasesRepo struct{ pool *pgxpool.Pool }

func NewReleasesRepo(pool *pgxpool.Pool) *ReleasesRepo { return &ReleasesRepo{pool: pool} }

func (r *ReleasesRepo) RegisterSource(ctx context.Context, in RegisterSourceInput) error {
	if in.RepoVisibility == "" {
		in.RepoVisibility = "public"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO image_source (repo_url, channel, repo_visibility, github_token_enc, registered_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (channel) DO UPDATE SET
		  repo_url         = EXCLUDED.repo_url,
		  repo_visibility  = EXCLUDED.repo_visibility,
		  github_token_enc = COALESCE(EXCLUDED.github_token_enc, image_source.github_token_enc)
	`, in.RepoURL, in.Channel, in.RepoVisibility, in.GithubTokenEnc, in.RegisteredBy)
	return err
}

func (r *ReleasesRepo) ListSources(ctx context.Context) ([]ImageSourceRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, repo_url, channel, repo_visibility, github_token_enc, registered_by, registered_at
		FROM image_source ORDER BY channel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImageSourceRow
	for rows.Next() {
		var s ImageSourceRow
		if err := rows.Scan(&s.ID, &s.RepoURL, &s.Channel, &s.RepoVisibility, &s.GithubTokenEnc, &s.RegisteredBy, &s.RegisteredAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ReleasesRepo) IngestRelease(ctx context.Context, in IngestReleaseInput) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO image_release
		  (channel, version, swu_url, swu_sha256, release_notes_md, release_html_url, release_published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (channel, version) DO NOTHING
	`, in.Channel, in.Version, in.SwuURL, in.SwuSha256, in.ReleaseNotesMD, in.ReleaseHTMLURL, in.ReleasePublishedAt)
	return err
}

func (r *ReleasesRepo) ListReleases(ctx context.Context, channel string) ([]ImageReleaseRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT channel, version, swu_url, swu_sha256, COALESCE(release_notes_md,''),
		       release_published_at, COALESCE(release_html_url,''), ingested_at
		FROM image_release WHERE channel = $1 ORDER BY ingested_at`, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImageReleaseRow
	for rows.Next() {
		var row ImageReleaseRow
		if err := rows.Scan(&row.Channel, &row.Version, &row.SwuURL, &row.SwuSha256, &row.ReleaseNotesMD,
			&row.ReleasePublishedAt, &row.ReleaseHTMLURL, &row.IngestedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// LatestRelease returns the highest-semver release for a channel, or (nil, nil)
// if the channel has none.
func (r *ReleasesRepo) LatestRelease(ctx context.Context, channel string) (*ImageReleaseRow, error) {
	releases, err := r.ListReleases(ctx, channel)
	if err != nil {
		return nil, err
	}
	var latest *ImageReleaseRow
	for i := range releases {
		if latest == nil || IsNewerVersionSemver(releases[i].Version, latest.Version) {
			latest = &releases[i]
		}
	}
	return latest, nil
}

// IsNewerVersionSemver reports whether candidate is a newer semver than other.
// The "v" prefix is optional. Exported so the images package reuses it.
func IsNewerVersionSemver(candidate, other string) bool {
	c, o := canonV(candidate), canonV(other)
	if !semver.IsValid(c) {
		return false
	}
	if !semver.IsValid(o) {
		return true
	}
	return semver.Compare(c, o) > 0
}

func canonV(v string) string {
	if len(v) == 0 || v[0] != 'v' {
		return "v" + v
	}
	return v
}
