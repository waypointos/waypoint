// Package alerts also contains the rover-local SQLite store that mirrors the
// alert lifecycle (raised → acknowledged → resolved). The proxy keeps its own
// authoritative copy in Postgres (see proxy/internal/db/alerts_repo.go), but
// the on-rover dashboard needs to render active alerts even when the proxy is
// offline — that's why the agent maintains a parallel store fed by its local
// NATS subject tree.
package alerts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Severity-string constants used in the local store and on the wire to the
// dashboard. Match the Postgres `alert_severity` enum on the proxy so the API
// layer can pass values straight through.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// State-string constants. Mirror the Postgres `alert_state` enum on the proxy.
const (
	StateActive       = "active"
	StateAcknowledged = "acknowledged"
	StateResolved     = "resolved"
)

// StoredAlert is the SQLite-native shape of an alert row. Pointer time fields
// distinguish "never set" (nil) from "set to zero time" — important for the
// dashboard, which renders "—" instead of an epoch timestamp for un-set
// acknowledged_at/resolved_at.
type StoredAlert struct {
	ID             string
	RoverID        string
	Severity       string // SeverityInfo | SeverityWarning | SeverityCritical
	Source         string
	Code           string
	Message        string
	Metadata       json.RawMessage // raw JSON; nil for "no metadata"
	RaisedAt       time.Time
	State          string // StateActive | StateAcknowledged | StateResolved
	AcknowledgedAt *time.Time
	AcknowledgedBy string
	ResolvedAt     *time.Time
	ResolvedBy     string
	AutoResolved   bool
}

// Store is a SQLite-backed mirror of the alert lifecycle. Rover-local; survives
// reboots. Uses modernc.org/sqlite (pure Go, no CGo) so the agent can be
// cross-compiled for the Pi without a C toolchain — same constraint as the
// audit store.
type Store struct {
	db *sql.DB
}

// OpenSQLite opens (or creates) the SQLite database at path. Use ":memory:"
// for tests. Schema is applied on every open.
//
// Connections are pinned to one — modernc.org/sqlite's `:memory:` database is
// per-connection, so without a max-of-one cap the schema applied on connection
// A is invisible to queries served by connection B. Even on disk-backed
// databases there's no benefit to parallel SQLite writers (SQLite serializes
// internally), so this is the right default everywhere.
func OpenSQLite(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := applyAlertsSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

func applyAlertsSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS alerts (
			id              TEXT PRIMARY KEY,
			rover_id        TEXT NOT NULL,
			severity        TEXT NOT NULL,
			source          TEXT NOT NULL,
			code            TEXT NOT NULL,
			message         TEXT NOT NULL,
			metadata        TEXT,
			raised_at       INTEGER NOT NULL,
			state           TEXT NOT NULL DEFAULT 'active',
			acknowledged_at INTEGER,
			acknowledged_by TEXT,
			resolved_at     INTEGER,
			resolved_by     TEXT,
			auto_resolved   INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS alerts_rover_state_idx ON alerts (rover_id, state, raised_at DESC);
	`)
	return err
}

// Raise inserts an active alert. If a row with the same id already exists
// (any state), the row is updated in place and forced back to 'active' with
// acknowledged/resolved fields cleared. This is intentional: the leaf-watcher's
// auto-resolve flow uses a deterministic id, so a leaf going down → up → down
// should surface as a fresh active alert.
func (s *Store) Raise(a StoredAlert) error {
	var meta any
	if len(a.Metadata) > 0 {
		meta = string(a.Metadata)
	}
	_, err := s.db.Exec(
		`INSERT INTO alerts (id, rover_id, severity, source, code, message, metadata, raised_at, state)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active')
		 ON CONFLICT(id) DO UPDATE SET
		     rover_id        = excluded.rover_id,
		     severity        = excluded.severity,
		     source          = excluded.source,
		     code            = excluded.code,
		     message         = excluded.message,
		     metadata        = excluded.metadata,
		     raised_at       = excluded.raised_at,
		     state           = 'active',
		     acknowledged_at = NULL,
		     acknowledged_by = NULL,
		     resolved_at     = NULL,
		     resolved_by     = NULL,
		     auto_resolved   = 0`,
		a.ID, a.RoverID, a.Severity, a.Source, a.Code, a.Message, meta, a.RaisedAt.Unix(),
	)
	return err
}

// Acknowledge transitions an alert to 'acknowledged'. Returns sql.ErrNoRows
// when the id is unknown so callers (HTTP handlers) can translate to 404.
func (s *Store) Acknowledge(id, userID string, at time.Time) error {
	res, err := s.db.Exec(
		`UPDATE alerts
		    SET state           = 'acknowledged',
		        acknowledged_at = ?,
		        acknowledged_by = ?
		  WHERE id = ?`,
		at.Unix(), nullableString(userID), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Resolve transitions an alert to 'resolved'. autoResolved=true marks
// programmatic resolution (leaf reconnect, first-boot of a new image) and
// userID is allowed to be empty in that case.
func (s *Store) Resolve(id, userID string, at time.Time, autoResolved bool) error {
	auto := 0
	if autoResolved {
		auto = 1
	}
	res, err := s.db.Exec(
		`UPDATE alerts
		    SET state         = 'resolved',
		        resolved_at   = ?,
		        resolved_by   = ?,
		        auto_resolved = ?
		  WHERE id = ?`,
		at.Unix(), nullableString(userID), auto, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListActive returns alerts where state != 'resolved' for the given rover,
// newest-raised first. roverID == "" means "all rovers".
func (s *Store) ListActive(roverID string) ([]StoredAlert, error) {
	q := `SELECT id, rover_id, severity, source, code, message, metadata, raised_at,
	             state, acknowledged_at, acknowledged_by, resolved_at, resolved_by, auto_resolved
	      FROM alerts
	      WHERE state != 'resolved'`
	var args []any
	if roverID != "" {
		q += ` AND rover_id = ?`
		args = append(args, roverID)
	}
	q += ` ORDER BY raised_at DESC`
	return s.queryRows(q, args...)
}

// GetHistory returns all alerts for the rover raised at-or-after `since`,
// newest first. roverID == "" means "all rovers".
func (s *Store) GetHistory(roverID string, since time.Time) ([]StoredAlert, error) {
	q := `SELECT id, rover_id, severity, source, code, message, metadata, raised_at,
	             state, acknowledged_at, acknowledged_by, resolved_at, resolved_by, auto_resolved
	      FROM alerts
	      WHERE raised_at >= ?`
	args := []any{since.Unix()}
	if roverID != "" {
		q += ` AND rover_id = ?`
		args = append(args, roverID)
	}
	q += ` ORDER BY raised_at DESC`
	return s.queryRows(q, args...)
}

func (s *Store) queryRows(q string, args ...any) ([]StoredAlert, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredAlert
	for rows.Next() {
		var a StoredAlert
		var raised int64
		var meta sql.NullString
		var ackAt sql.NullInt64
		var ackBy sql.NullString
		var resAt sql.NullInt64
		var resBy sql.NullString
		var auto int64
		if err := rows.Scan(
			&a.ID, &a.RoverID, &a.Severity, &a.Source, &a.Code, &a.Message,
			&meta, &raised, &a.State,
			&ackAt, &ackBy, &resAt, &resBy, &auto,
		); err != nil {
			return nil, err
		}
		a.RaisedAt = time.Unix(raised, 0).UTC()
		if meta.Valid {
			a.Metadata = json.RawMessage(meta.String)
		}
		if ackAt.Valid {
			t := time.Unix(ackAt.Int64, 0).UTC()
			a.AcknowledgedAt = &t
		}
		if ackBy.Valid {
			a.AcknowledgedBy = ackBy.String
		}
		if resAt.Valid {
			t := time.Unix(resAt.Int64, 0).UTC()
			a.ResolvedAt = &t
		}
		if resBy.Valid {
			a.ResolvedBy = resBy.String
		}
		a.AutoResolved = auto != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SeverityToString maps the protobuf Severity enum to the canonical string
// stored in both SQLite and Postgres. Lives here (not the publisher) so the
// agent local subscriber and proxy mirror both share one source of truth via
// duplication-with-test-coverage — the alternative would be a shared module
// these two packages don't have today. Defensive default for UNSPECIFIED is
// "info", matching the proxy enum's least-noisy value.
//
// We accept an int32 instead of waypointv1.Severity so this file doesn't have
// to import the protobuf bindings — keeps the store package buildable in
// isolation.
func SeverityToString(v int32) string {
	switch v {
	case 1: // SEVERITY_INFO
		return SeverityInfo
	case 2: // SEVERITY_WARN
		return SeverityWarning
	case 3: // SEVERITY_FAULT
		return SeverityCritical
	default:
		return SeverityInfo
	}
}

// ErrNotFound is exported for callers that prefer pattern-matching over
// sql.ErrNoRows. Both work — Acknowledge and Resolve return sql.ErrNoRows,
// which errors.Is against this sentinel for callers that import only this
// package.
var ErrNotFound = errors.New("alert not found")
