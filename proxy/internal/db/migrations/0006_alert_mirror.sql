-- +goose Up
CREATE TYPE alert_severity AS ENUM ('info', 'warning', 'critical');
CREATE TYPE alert_state    AS ENUM ('active', 'acknowledged', 'resolved');

CREATE TABLE alerts (
    id              TEXT PRIMARY KEY,
    rover_id        TEXT NOT NULL REFERENCES rovers(id) ON DELETE CASCADE,
    severity        alert_severity NOT NULL,
    source          TEXT NOT NULL,
    code            TEXT NOT NULL,
    message         TEXT NOT NULL,
    metadata        JSONB,
    raised_at       TIMESTAMPTZ NOT NULL,
    state           alert_state NOT NULL DEFAULT 'active',
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID REFERENCES users(id),
    resolved_at     TIMESTAMPTZ,
    resolved_by     UUID REFERENCES users(id),
    auto_resolved   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX alerts_rover_state_idx ON alerts (rover_id, state, raised_at DESC);

-- +goose Down
DROP TABLE alerts;
DROP TYPE alert_state;
DROP TYPE alert_severity;
