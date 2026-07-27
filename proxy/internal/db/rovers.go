package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Rover struct {
	ID               string
	Name             string
	AccountPubKey    string
	EnrolledByUserID uuid.UUID
	EnrolledAt       time.Time
	LastSeenAt       *time.Time
	ImageVersion     *string
	// AccountJWT + AccountSeed persist the rover's NATS account credentials
	// across proxy restarts (in-memory MemAccResolver + operator keypair
	// cache are both wiped on restart). Both are nullable for rows that
	// existed before migration 0007 — those rovers cannot be recovered
	// without re-enrollment.
	AccountJWT  *string
	AccountSeed *string
}

type RoversRepo struct{ pool *pgxpool.Pool }

func NewRoversRepo(p *pgxpool.Pool) *RoversRepo { return &RoversRepo{pool: p} }

func (r *RoversRepo) Create(ctx context.Context, id, name, accountPubkey, accountJWT, accountSeed string, enrolledBy uuid.UUID) (*Rover, error) {
	var rv Rover
	err := r.pool.QueryRow(ctx,
		`INSERT INTO rovers (id, name, account_pubkey, account_jwt, account_seed, enrolled_by_user_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, account_pubkey, account_jwt, account_seed, enrolled_by_user_id, enrolled_at, last_seen_at, image_version`,
		id, name, accountPubkey, accountJWT, accountSeed, enrolledBy,
	).Scan(&rv.ID, &rv.Name, &rv.AccountPubKey, &rv.AccountJWT, &rv.AccountSeed, &rv.EnrolledByUserID, &rv.EnrolledAt, &rv.LastSeenAt, &rv.ImageVersion)
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

func (r *RoversRepo) Get(ctx context.Context, id string) (*Rover, error) {
	var rv Rover
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, account_pubkey, account_jwt, account_seed, enrolled_by_user_id, enrolled_at, last_seen_at, image_version
		 FROM rovers WHERE id = $1`, id,
	).Scan(&rv.ID, &rv.Name, &rv.AccountPubKey, &rv.AccountJWT, &rv.AccountSeed, &rv.EnrolledByUserID, &rv.EnrolledAt, &rv.LastSeenAt, &rv.ImageVersion)
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

func (r *RoversRepo) List(ctx context.Context) ([]Rover, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, account_pubkey, account_jwt, account_seed, enrolled_by_user_id, enrolled_at, last_seen_at, image_version
		 FROM rovers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rover
	for rows.Next() {
		var rv Rover
		if err := rows.Scan(&rv.ID, &rv.Name, &rv.AccountPubKey, &rv.AccountJWT, &rv.AccountSeed, &rv.EnrolledByUserID, &rv.EnrolledAt, &rv.LastSeenAt, &rv.ImageVersion); err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

func (r *RoversRepo) UpdateLastSeen(ctx context.Context, id string, t time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE rovers SET last_seen_at = $2 WHERE id = $1`, id, t)
	return err
}

func (r *RoversRepo) UpdateImageVersion(ctx context.Context, id, version string) error {
	_, err := r.pool.Exec(ctx, `UPDATE rovers SET image_version = $2 WHERE id = $1`, id, version)
	return err
}

// Delete removes the rover row. FK cascades drop user_rover_access,
// image_apply_history, and alerts. audit_events.rover_id is a free-text
// column (not a FK), so historical audit rows referencing this rover survive.
func (r *RoversRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rovers WHERE id = $1`, id)
	return err
}
