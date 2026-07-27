package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Default retention limits applied by Sweep. The rover-local audit log is
// best-effort — long-term retention is the proxy's job (Postgres). 30 days
// or 50k rows covers the "what did the rover do recently?" use case while
// staying well within a Pi's storage budget.
const (
	DefaultMaxAge  = 30 * 24 * time.Hour
	DefaultMaxRows = 50_000
)

// Store is a SQLite-backed audit log. Rover-local; survives reboots.
// Uses modernc.org/sqlite (pure Go, no CGo) so we can cross-compile the
// agent for the Pi without a C toolchain.
type Store struct {
	db *sql.DB
}

// OpenSQLite opens (or creates) the SQLite database at path. Use ":memory:"
// for tests. Schema is applied on every open (CREATE TABLE IF NOT EXISTS).
//
// Connections are pinned to one because modernc.org/sqlite's `:memory:` is
// per-connection: schema applied on connection A is invisible to queries
// served by connection B. SQLite serializes writes internally anyway, so
// there's no benefit to parallel writers — this is the right default whether
// the path is ":memory:" or a disk-backed file.
func OpenSQLite(path string) (*Store, error) {
	// modernc.org/sqlite registers itself as driver name "sqlite" (not
	// "sqlite3" — that's mattn's CGo driver).
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := applySchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func applySchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_events (
			id            TEXT PRIMARY KEY,
			occurred_at   INTEGER NOT NULL,                -- unix seconds
			source        TEXT    NOT NULL,
			actor_user_id TEXT,
			actor_email   TEXT,
			rover_id      TEXT    NOT NULL,
			kind          TEXT    NOT NULL,
			payload       TEXT    NOT NULL,                -- JSON
			received_at   INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		);
		CREATE INDEX IF NOT EXISTS audit_rover_idx ON audit_events (rover_id, occurred_at DESC);
	`)
	return err
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// Row mirrors the proxy's AuditRow but with SQLite-native scalar types.
// Empty ActorUserID is encoded as NULL in the database (system-issued event).
type Row struct {
	ID          string
	OccurredAt  time.Time
	Source      string
	ActorUserID string // empty = NULL
	ActorEmail  string
	RoverID     string
	Kind        string
	Payload     json.RawMessage
}

// Filter narrows List results. Zero values mean "no filter."
type Filter struct {
	RoverID   string
	SinceTime *time.Time
	Limit     int
}

// Insert writes a row to the audit log. Duplicate IDs are silently ignored
// (ON CONFLICT DO NOTHING) so a redelivered NATS message doesn't create
// duplicate audit entries.
func (s *Store) Insert(row Row) error {
	var actor any
	if row.ActorUserID != "" {
		actor = row.ActorUserID
	}
	_, err := s.db.Exec(
		`INSERT INTO audit_events (id, occurred_at, source, actor_user_id, actor_email, rover_id, kind, payload)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		row.ID, row.OccurredAt.Unix(), row.Source, actor, row.ActorEmail, row.RoverID, row.Kind, string(row.Payload),
	)
	return err
}

// List returns audit rows matching the filter, newest first. If Limit is
// zero or negative, defaults to 500.
func (s *Store) List(f Filter) ([]Row, error) {
	if f.Limit <= 0 {
		f.Limit = 500
	}
	args := []any{}
	where := ""
	if f.RoverID != "" {
		where += " WHERE rover_id = ?"
		args = append(args, f.RoverID)
	}
	if f.SinceTime != nil {
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		where += "occurred_at >= ?"
		args = append(args, f.SinceTime.Unix())
	}
	args = append(args, f.Limit)
	q := `SELECT id, occurred_at, source, actor_user_id, actor_email, rover_id, kind, payload
	      FROM audit_events` + where + ` ORDER BY occurred_at DESC LIMIT ?`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var occurred int64
		var actor sql.NullString
		var email sql.NullString
		var payload string
		if err := rows.Scan(&r.ID, &occurred, &r.Source, &actor, &email, &r.RoverID, &r.Kind, &payload); err != nil {
			return nil, err
		}
		// Be explicit about UTC: the rover may run in a non-UTC TZ.
		r.OccurredAt = time.Unix(occurred, 0).UTC()
		if actor.Valid {
			r.ActorUserID = actor.String
		}
		if email.Valid {
			r.ActorEmail = email.String
		}
		r.Payload = json.RawMessage(payload)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Sweep removes rows older than maxAge AND trims to maxRows (whichever is more
// aggressive). Returns the number of rows deleted.
//
// Called periodically by the local subscriber; safe to call from one goroutine
// while inserts happen on another (SQLite serializes writes internally).
func (s *Store) Sweep(maxAge time.Duration, maxRows int) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	res, err := s.db.Exec(`DELETE FROM audit_events WHERE occurred_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	byAge, _ := res.RowsAffected()
	// After the age sweep, enforce the row-count cap. `LIMIT -1 OFFSET ?`
	// means "skip first N, no upper limit" — SQLite-specific.
	res, err = s.db.Exec(
		`DELETE FROM audit_events
		 WHERE id IN (
		     SELECT id FROM audit_events
		     ORDER BY occurred_at DESC
		     LIMIT -1 OFFSET ?
		 )`,
		maxRows,
	)
	if err != nil {
		return byAge, err
	}
	byCount, _ := res.RowsAffected()
	return byAge + byCount, nil
}
