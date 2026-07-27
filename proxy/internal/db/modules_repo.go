package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ModuleRow struct {
	ModuleID            string
	DisplayName         string
	SourceRepoURL       string
	ExpectedIdentitySAN string
	RegisteredBy        uuid.UUID
	RegisteredAt        time.Time
	RepoVisibility      string
	GithubTokenEnc      []byte
}

type VersionRow struct {
	ModuleID         string
	Version          string
	RawPath          string
	RawSha256        string
	ManifestJSON     []byte
	IngestedAt       time.Time
	ReleaseNotesMD string
	ReleaseHTMLURL string
}

type DesiredRow struct {
	RoverID        string
	ModuleID       string
	DesiredVersion string
	ConfigTOML     string
	UpdatedBy      uuid.UUID
	UpdatedAt      time.Time
}

type RoverModuleRow struct {
	RoverID        string
	DesiredVersion string
	AutoUpdate     bool
}

type RegisterModuleInput struct {
	ModuleID, DisplayName, SourceRepoURL string
	ExpectedIdentitySAN                  string
	RegisteredBy                         uuid.UUID
	RepoVisibility                       string
	GithubTokenEnc                       []byte
}

type IngestVersionInput struct {
	ModuleID, Version, RawPath, RawSha256 string
	ManifestJSON                          []byte
	ReleaseNotesMD                        string
	ReleaseHTMLURL                        string
}

type SetDesiredInput struct {
	RoverID, ModuleID, DesiredVersion, ConfigTOML string
	UpdatedBy                                     uuid.UUID
}

type ModuleWithVersions struct {
	Module   ModuleRow
	Versions []VersionRow
}

type ModulesRepo struct {
	pool *pgxpool.Pool
}

func NewModulesRepo(pool *pgxpool.Pool) *ModulesRepo {
	return &ModulesRepo{pool: pool}
}

func (r *ModulesRepo) RegisterModule(ctx context.Context, in RegisterModuleInput) error {
	if in.RepoVisibility == "" {
		in.RepoVisibility = "public"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO module_registry
		  (module_id, display_name, source_repo_url, expected_identity_san, registered_by, repo_visibility, github_token_enc)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (module_id) DO UPDATE SET
		  display_name          = EXCLUDED.display_name,
		  source_repo_url       = EXCLUDED.source_repo_url,
		  expected_identity_san = EXCLUDED.expected_identity_san,
		  repo_visibility       = EXCLUDED.repo_visibility,
		  github_token_enc      = COALESCE(EXCLUDED.github_token_enc, module_registry.github_token_enc)
	`, in.ModuleID, in.DisplayName, in.SourceRepoURL, in.ExpectedIdentitySAN, in.RegisteredBy, in.RepoVisibility, in.GithubTokenEnc)
	return err
}

func (r *ModulesRepo) GetModule(ctx context.Context, moduleID string) (*ModuleRow, error) {
	var m ModuleRow
	err := r.pool.QueryRow(ctx, `
		SELECT module_id, display_name, source_repo_url, expected_identity_san, registered_by, registered_at,
		       repo_visibility, github_token_enc
		FROM module_registry WHERE module_id = $1
	`, moduleID).Scan(&m.ModuleID, &m.DisplayName, &m.SourceRepoURL, &m.ExpectedIdentitySAN, &m.RegisteredBy, &m.RegisteredAt,
		&m.RepoVisibility, &m.GithubTokenEnc)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *ModulesRepo) IngestVersion(ctx context.Context, in IngestVersionInput) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO module_versions
		  (module_id, version, raw_path, raw_sha256, manifest_json, release_notes_md, release_html_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (module_id, version) DO NOTHING
	`, in.ModuleID, in.Version, in.RawPath, in.RawSha256, in.ManifestJSON, in.ReleaseNotesMD, in.ReleaseHTMLURL)
	return err
}

func (r *ModulesRepo) ListVersions(ctx context.Context, moduleID string) ([]VersionRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT module_id, version, raw_path, raw_sha256, manifest_json, ingested_at,
		       COALESCE(release_notes_md,''), COALESCE(release_html_url,'')
		FROM module_versions WHERE module_id = $1 ORDER BY ingested_at DESC
	`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionRow
	for rows.Next() {
		var v VersionRow
		if err := rows.Scan(&v.ModuleID, &v.Version, &v.RawPath, &v.RawSha256, &v.ManifestJSON, &v.IngestedAt,
			&v.ReleaseNotesMD, &v.ReleaseHTMLURL); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *ModulesRepo) SetDesired(ctx context.Context, in SetDesiredInput) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO rover_module_state (rover_id, module_id, desired_version, config_toml, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (rover_id, module_id) DO UPDATE SET
		  desired_version = EXCLUDED.desired_version,
		  config_toml     = EXCLUDED.config_toml,
		  updated_by      = EXCLUDED.updated_by,
		  updated_at      = NOW()
	`, in.RoverID, in.ModuleID, in.DesiredVersion, in.ConfigTOML, in.UpdatedBy)
	return err
}

func (r *ModulesRepo) DesiredForRover(ctx context.Context, roverID string) ([]DesiredRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rover_id, module_id, COALESCE(desired_version,''), COALESCE(config_toml,''), updated_by, updated_at
		FROM rover_module_state WHERE rover_id = $1
	`, roverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DesiredRow
	for rows.Next() {
		var d DesiredRow
		if err := rows.Scan(&d.RoverID, &d.ModuleID, &d.DesiredVersion, &d.ConfigTOML, &d.UpdatedBy, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Unpin removes one rover-module desired pairing. The next desired-set push
// omits the module, so the rover's reconciler detaches and stops it.
func (r *ModulesRepo) Unpin(ctx context.Context, roverID, moduleID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rover_module_state WHERE rover_id = $1 AND module_id = $2`, roverID, moduleID)
	return err
}

// DeleteModule removes a module from the registry; versions and rover
// pairings go with it via FK cascade. This also forgets the TOFU identity
// pin, so delete + re-register is the sanctioned way to re-point a module id
// at a different repo.
func (r *ModulesRepo) DeleteModule(ctx context.Context, moduleID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM module_registry WHERE module_id = $1`, moduleID)
	return err
}

func (r *ModulesRepo) SetReadme(ctx context.Context, moduleID, html string) error {
	_, err := r.pool.Exec(ctx, `UPDATE module_registry SET readme_md = $2, readme_fetched_at = NOW() WHERE module_id = $1`, moduleID, html)
	return err
}

func (r *ModulesRepo) GetReadme(ctx context.Context, moduleID string) (string, error) {
	var html string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(readme_md,'') FROM module_registry WHERE module_id = $1`, moduleID).Scan(&html)
	return html, err
}

func (r *ModulesRepo) SetAutoUpdate(ctx context.Context, roverID, moduleID string, auto bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE rover_module_state SET auto_update = $3, updated_at = NOW() WHERE rover_id = $1 AND module_id = $2`, roverID, moduleID, auto)
	return err
}

func (r *ModulesRepo) RoversForModule(ctx context.Context, moduleID string) ([]RoverModuleRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT rover_id, desired_version, auto_update FROM rover_module_state WHERE module_id = $1 ORDER BY rover_id`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoverModuleRow
	for rows.Next() {
		var rm RoverModuleRow
		if err := rows.Scan(&rm.RoverID, &rm.DesiredVersion, &rm.AutoUpdate); err != nil {
			return nil, err
		}
		out = append(out, rm)
	}
	return out, rows.Err()
}

func (r *ModulesRepo) ListAll(ctx context.Context) ([]ModuleWithVersions, error) {
	rows, err := r.pool.Query(ctx, `SELECT module_id FROM module_registry ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]ModuleWithVersions, 0, len(ids))
	for _, id := range ids {
		m, err := r.GetModule(ctx, id)
		if err != nil {
			return nil, err
		}
		vs, err := r.ListVersions(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ModuleWithVersions{Module: *m, Versions: vs})
	}
	return out, nil
}
