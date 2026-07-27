package db

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AlertRow is the wire-format-agnostic shape of a row in the alerts table.
// State, AcknowledgedAt/By, ResolvedAt/By are pointers because they're
// optional in the lifecycle.
type AlertRow struct {
	ID             string
	RoverID        string
	Severity       string // "info" | "warning" | "critical"
	Source         string
	Code           string
	Message        string
	Metadata       json.RawMessage // empty/nil → stored as SQL NULL
	RaisedAt       time.Time
	State          string // "active" | "acknowledged" | "resolved"
	AcknowledgedAt *time.Time
	AcknowledgedBy *uuid.UUID
	ResolvedAt     *time.Time
	ResolvedBy     *uuid.UUID
	AutoResolved   bool
}

// AlertsRepo persists active/historical alerts. The proxy is the
// authoritative copy — the rover keeps a parallel SQLite mirror for offline
// rendering (see agent/internal/alerts/store.go).
type AlertsRepo struct{ pool *pgxpool.Pool }

func NewAlertsRepo(p *pgxpool.Pool) *AlertsRepo { return &AlertsRepo{pool: p} }

// Raise inserts an active alert. On conflict (same id), the row is updated in
// place and forced back to 'active' with acknowledged/resolved fields cleared.
// Matches the SQLite store's idempotency contract: the leaf-watcher's
// deterministic ids must surface as a fresh alert on re-raise.
func (r *AlertsRepo) Raise(ctx context.Context, a AlertRow) error {
	var meta any
	if len(a.Metadata) > 0 {
		meta = a.Metadata
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO alerts
		    (id, rover_id, severity, source, code, message, metadata, raised_at, state)
		 VALUES ($1, $2, $3::alert_severity, $4, $5, $6, $7, $8, 'active'::alert_state)
		 ON CONFLICT (id) DO UPDATE SET
		     rover_id        = EXCLUDED.rover_id,
		     severity        = EXCLUDED.severity,
		     source          = EXCLUDED.source,
		     code            = EXCLUDED.code,
		     message         = EXCLUDED.message,
		     metadata        = EXCLUDED.metadata,
		     raised_at       = EXCLUDED.raised_at,
		     state           = 'active'::alert_state,
		     acknowledged_at = NULL,
		     acknowledged_by = NULL,
		     resolved_at     = NULL,
		     resolved_by     = NULL,
		     auto_resolved   = FALSE`,
		a.ID, a.RoverID, a.Severity, a.Source, a.Code, a.Message, meta, a.RaisedAt,
	)
	return err
}

// Acknowledge transitions an alert to 'acknowledged'. Returns pgx.ErrNoRows
// when the id is unknown.
func (r *AlertsRepo) Acknowledge(ctx context.Context, id string, userID uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE alerts
		    SET state           = 'acknowledged'::alert_state,
		        acknowledged_at = $2,
		        acknowledged_by = $3
		  WHERE id = $1`,
		id, at, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Resolve transitions an alert to 'resolved'. userID may be nil when
// autoResolved is true — that's the leaf-watcher auto-resolve case.
func (r *AlertsRepo) Resolve(ctx context.Context, id string, userID *uuid.UUID, at time.Time, autoResolved bool) error {
	var by any
	if userID != nil {
		by = *userID
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE alerts
		    SET state         = 'resolved'::alert_state,
		        resolved_at   = $2,
		        resolved_by   = $3,
		        auto_resolved = $4
		  WHERE id = $1`,
		id, at, by, autoResolved,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetByID fetches a single alert by id. Returns pgx.ErrNoRows when the id is
// unknown. Used by the alert API handlers to authorize ack/resolve against the
// alert's rover before mutating state.
func (r *AlertsRepo) GetByID(ctx context.Context, id string) (AlertRow, error) {
	var a AlertRow
	var meta []byte
	var ackAt *time.Time
	var ackBy *uuid.UUID
	var resAt *time.Time
	var resBy *uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT id, rover_id, severity::text, source, code, message, metadata, raised_at,
		        state::text, acknowledged_at, acknowledged_by, resolved_at, resolved_by, auto_resolved
		   FROM alerts
		  WHERE id = $1`, id,
	).Scan(
		&a.ID, &a.RoverID, &a.Severity, &a.Source, &a.Code, &a.Message,
		&meta, &a.RaisedAt, &a.State,
		&ackAt, &ackBy, &resAt, &resBy, &a.AutoResolved,
	)
	if err != nil {
		return AlertRow{}, err
	}
	if len(meta) > 0 {
		a.Metadata = json.RawMessage(meta)
	}
	a.AcknowledgedAt = ackAt
	a.AcknowledgedBy = ackBy
	a.ResolvedAt = resAt
	a.ResolvedBy = resBy
	return a, nil
}

// ListActive returns alerts where state != 'resolved' for the given rover,
// newest-raised first. roverID == "" means "all rovers".
func (r *AlertsRepo) ListActive(ctx context.Context, roverID string) ([]AlertRow, error) {
	q := `SELECT id, rover_id, severity::text, source, code, message, metadata, raised_at,
	             state::text, acknowledged_at, acknowledged_by, resolved_at, resolved_by, auto_resolved
	        FROM alerts
	       WHERE state != 'resolved'::alert_state`
	args := []any{}
	if roverID != "" {
		args = append(args, roverID)
		q += ` AND rover_id = $` + strconv.Itoa(len(args))
	}
	q += ` ORDER BY raised_at DESC`
	return r.queryRows(ctx, q, args...)
}

// GetHistory returns all alerts for the rover raised at-or-after `since`,
// newest first. roverID == "" means "all rovers".
func (r *AlertsRepo) GetHistory(ctx context.Context, roverID string, since time.Time) ([]AlertRow, error) {
	q := `SELECT id, rover_id, severity::text, source, code, message, metadata, raised_at,
	             state::text, acknowledged_at, acknowledged_by, resolved_at, resolved_by, auto_resolved
	        FROM alerts
	       WHERE raised_at >= $1`
	args := []any{since}
	if roverID != "" {
		args = append(args, roverID)
		q += ` AND rover_id = $` + strconv.Itoa(len(args))
	}
	q += ` ORDER BY raised_at DESC`
	return r.queryRows(ctx, q, args...)
}

func (r *AlertsRepo) queryRows(ctx context.Context, q string, args ...any) ([]AlertRow, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlertRow
	for rows.Next() {
		var a AlertRow
		var meta []byte
		var ackAt *time.Time
		var ackBy *uuid.UUID
		var resAt *time.Time
		var resBy *uuid.UUID
		if err := rows.Scan(
			&a.ID, &a.RoverID, &a.Severity, &a.Source, &a.Code, &a.Message,
			&meta, &a.RaisedAt, &a.State,
			&ackAt, &ackBy, &resAt, &resBy, &a.AutoResolved,
		); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			a.Metadata = json.RawMessage(meta)
		}
		a.AcknowledgedAt = ackAt
		a.AcknowledgedBy = ackBy
		a.ResolvedAt = resAt
		a.ResolvedBy = resBy
		out = append(out, a)
	}
	return out, rows.Err()
}

// ErrAlertNotFound is exported for callers preferring a typed sentinel.
// AlertsRepo.Acknowledge and Resolve return pgx.ErrNoRows directly; both
// errors.Is against this sentinel.
var ErrAlertNotFound = errors.New("alert not found")
