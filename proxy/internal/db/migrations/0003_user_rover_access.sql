-- +goose Up
CREATE TYPE rover_role AS ENUM ('monitor', 'control', 'admin');

CREATE TABLE user_rover_access (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rover_id    TEXT NOT NULL REFERENCES rovers(id) ON DELETE CASCADE,
    role        rover_role NOT NULL,
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    granted_by  UUID NOT NULL REFERENCES users(id),
    PRIMARY KEY (user_id, rover_id)
);

CREATE INDEX user_rover_access_rover_idx ON user_rover_access (rover_id);

-- +goose Down
DROP TABLE user_rover_access;
DROP TYPE rover_role;
