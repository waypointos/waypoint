-- +goose Up
CREATE TABLE image_source (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_url         TEXT NOT NULL,
    channel          TEXT NOT NULL,
    repo_visibility  TEXT NOT NULL DEFAULT 'public',
    github_token_enc BYTEA,
    registered_by    UUID NOT NULL REFERENCES users(id),
    registered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel)
);

CREATE TABLE image_release (
    channel              TEXT NOT NULL,
    version              TEXT NOT NULL,
    swu_url              TEXT NOT NULL,
    swu_sha256           TEXT NOT NULL DEFAULT '',
    release_notes_md     TEXT,
    release_published_at TIMESTAMPTZ,
    release_html_url     TEXT,
    ingested_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel, version)
);

-- +goose Down
DROP TABLE image_release;
DROP TABLE image_source;
