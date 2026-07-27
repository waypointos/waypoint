-- +goose Up
--
-- No-rebuild module registry.

CREATE TABLE module_registry (
    module_id        TEXT PRIMARY KEY,
    display_name     TEXT NOT NULL,
    source_repo_url  TEXT NOT NULL,
    signing_key_pub  TEXT NOT NULL,
    signing_key_fpr  TEXT NOT NULL UNIQUE,
    registered_by    UUID NOT NULL REFERENCES users(id),
    registered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE module_versions (
    module_id        TEXT NOT NULL REFERENCES module_registry(module_id) ON DELETE CASCADE,
    version          TEXT NOT NULL,
    raw_path         TEXT NOT NULL,
    raw_sha256       TEXT NOT NULL,
    manifest_json    JSONB NOT NULL,
    ingested_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (module_id, version)
);

CREATE TABLE rover_module_state (
    rover_id         TEXT NOT NULL REFERENCES rovers(id) ON DELETE CASCADE,
    module_id        TEXT NOT NULL REFERENCES module_registry(module_id) ON DELETE CASCADE,
    desired_version  TEXT,
    config_toml      TEXT,
    updated_by       UUID REFERENCES users(id),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rover_id, module_id)
);

CREATE INDEX module_versions_by_module ON module_versions (module_id, ingested_at DESC);
CREATE INDEX rover_module_state_by_rover ON rover_module_state (rover_id);

-- +goose Down
DROP TABLE rover_module_state;
DROP TABLE module_versions;
DROP TABLE module_registry;
