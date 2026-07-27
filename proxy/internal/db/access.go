package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Access struct {
	UserID  uuid.UUID
	RoverID string
	Role    string
}

type AccessRepo struct{ pool *pgxpool.Pool }

func NewAccessRepo(p *pgxpool.Pool) *AccessRepo { return &AccessRepo{pool: p} }

func (a *AccessRepo) Grant(ctx context.Context, userID uuid.UUID, roverID, role string, grantedBy uuid.UUID) error {
	_, err := a.pool.Exec(ctx,
		`INSERT INTO user_rover_access (user_id, rover_id, role, granted_by)
		 VALUES ($1, $2, $3::rover_role, $4)
		 ON CONFLICT (user_id, rover_id) DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by, granted_at = NOW()`,
		userID, roverID, role, grantedBy)
	return err
}

func (a *AccessRepo) Revoke(ctx context.Context, userID uuid.UUID, roverID string) error {
	_, err := a.pool.Exec(ctx, `DELETE FROM user_rover_access WHERE user_id=$1 AND rover_id=$2`, userID, roverID)
	return err
}

func (a *AccessRepo) RoleFor(ctx context.Context, userID uuid.UUID, roverID string) (string, error) {
	var role string
	err := a.pool.QueryRow(ctx,
		`SELECT role::text FROM user_rover_access WHERE user_id=$1 AND rover_id=$2`,
		userID, roverID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// EffectiveRole returns "admin" if isAdmin is true.
// Otherwise it returns the explicit grant or "" if none.
func (a *AccessRepo) EffectiveRole(ctx context.Context, userID uuid.UUID, roverID string, isAdmin bool) (string, error) {
	if isAdmin {
		return "admin", nil
	}
	return a.RoleFor(ctx, userID, roverID)
}

func (a *AccessRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]Access, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT user_id, rover_id, role::text FROM user_rover_access WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Access
	for rows.Next() {
		var ac Access
		if err := rows.Scan(&ac.UserID, &ac.RoverID, &ac.Role); err != nil {
			return nil, err
		}
		out = append(out, ac)
	}
	return out, rows.Err()
}
