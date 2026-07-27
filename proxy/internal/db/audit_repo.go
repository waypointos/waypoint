package db

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRow is the wire-format-agnostic shape of a row in audit_events.
// ActorUserID is a pointer because system-issued events (e.g. agent-issued
// ImageApplied) have no human actor.
type AuditRow struct {
	ID          uuid.UUID
	OccurredAt  time.Time
	Source      string
	ActorUserID *uuid.UUID
	ActorEmail  string
	RoverID     string
	Kind        string
	Payload     json.RawMessage
}

// AuditFilter narrows AuditRepo.List. Zero values mean "no filter".
type AuditFilter struct {
	RoverID     string     // empty -> no filter
	ActorUserID *uuid.UUID // nil -> no filter
	SinceTime   *time.Time // nil -> no filter
	Limit       int        // 0 -> default 500
}

type AuditRepo struct{ pool *pgxpool.Pool }

func NewAuditRepo(p *pgxpool.Pool) *AuditRepo { return &AuditRepo{pool: p} }

// Insert persists a single audit row. Duplicate IDs are silently ignored so
// that re-delivered NATS messages don't double-count.
func (r *AuditRepo) Insert(ctx context.Context, row AuditRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_events (id, occurred_at, source, actor_user_id, actor_email, rover_id, kind, payload)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO NOTHING`,
		row.ID, row.OccurredAt, row.Source, row.ActorUserID, row.ActorEmail, row.RoverID, row.Kind, row.Payload,
	)
	return err
}

// List returns rows matching the filter in descending occurred_at order.
func (r *AuditRepo) List(ctx context.Context, f AuditFilter) ([]AuditRow, error) {
	if f.Limit <= 0 {
		f.Limit = 500
	}

	args := []any{}
	where := ""
	add := func(clause string, v any) {
		args = append(args, v)
		if where == "" {
			where = "WHERE "
		} else {
			where += " AND "
		}
		where += clause
	}
	if f.RoverID != "" {
		add("rover_id = $"+strconv.Itoa(len(args)+1), f.RoverID)
	}
	if f.ActorUserID != nil {
		add("actor_user_id = $"+strconv.Itoa(len(args)+1), *f.ActorUserID)
	}
	if f.SinceTime != nil {
		add("occurred_at >= $"+strconv.Itoa(len(args)+1), *f.SinceTime)
	}

	q := `SELECT id, occurred_at, source, actor_user_id, actor_email, rover_id, kind, payload
	      FROM audit_events ` + where + ` ORDER BY occurred_at DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, f.Limit)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditRow
	for rows.Next() {
		var row AuditRow
		var actor *uuid.UUID
		var actorEmail *string
		if err := rows.Scan(&row.ID, &row.OccurredAt, &row.Source, &actor, &actorEmail, &row.RoverID, &row.Kind, &row.Payload); err != nil {
			return nil, err
		}
		row.ActorUserID = actor
		if actorEmail != nil {
			row.ActorEmail = *actorEmail
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
