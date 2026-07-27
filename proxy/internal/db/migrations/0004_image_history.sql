-- +goose Up
CREATE TABLE image_apply_history (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rover_id     TEXT NOT NULL REFERENCES rovers(id) ON DELETE CASCADE,
    url          TEXT NOT NULL,
    version      TEXT,
    requested_by UUID NOT NULL REFERENCES users(id),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status       TEXT NOT NULL DEFAULT 'pending'
);

CREATE INDEX image_apply_history_rover_idx ON image_apply_history (rover_id, requested_at DESC);

-- +goose Down
DROP TABLE image_apply_history;
