-- +goose Up
ALTER TABLE module_registry
  ADD COLUMN repo_visibility   TEXT NOT NULL DEFAULT 'public',
  ADD COLUMN github_token_enc  BYTEA,
  ADD COLUMN readme_md         TEXT,
  ADD COLUMN readme_fetched_at TIMESTAMPTZ;

ALTER TABLE module_versions
  ADD COLUMN release_notes_md     TEXT,
  ADD COLUMN release_published_at TIMESTAMPTZ,
  ADD COLUMN release_html_url     TEXT;

ALTER TABLE rover_module_state
  ADD COLUMN auto_update BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE rover_module_state DROP COLUMN auto_update;
ALTER TABLE module_versions
  DROP COLUMN release_notes_md,
  DROP COLUMN release_published_at,
  DROP COLUMN release_html_url;
ALTER TABLE module_registry
  DROP COLUMN repo_visibility,
  DROP COLUMN github_token_enc,
  DROP COLUMN readme_md,
  DROP COLUMN readme_fetched_at;
