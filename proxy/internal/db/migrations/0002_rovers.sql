-- +goose Up
CREATE TABLE rovers (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    account_pubkey      TEXT NOT NULL,
    enrolled_by_user_id UUID NOT NULL REFERENCES users(id),
    enrolled_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ,
    image_version       TEXT
);

CREATE INDEX rovers_last_seen_idx ON rovers (last_seen_at);

-- +goose Down
DROP TABLE rovers;
