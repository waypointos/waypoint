-- +goose Up
CREATE TABLE audit_events (
    id              UUID PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL,
    source          TEXT NOT NULL,
    actor_user_id   UUID REFERENCES users(id),
    actor_email     TEXT,
    rover_id        TEXT NOT NULL,
    kind            TEXT NOT NULL,           -- 'command' | 'access' | 'session_open' | 'session_close' | 'image'
    payload         JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_events_rover_at_idx ON audit_events (rover_id, occurred_at DESC);
CREATE INDEX audit_events_actor_at_idx ON audit_events (actor_user_id, occurred_at DESC);

-- +goose Down
DROP TABLE audit_events;
