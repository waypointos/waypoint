package db

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           uuid.UUID
	WorkOSUserID string
	Email        string
	IsAdmin      bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type UsersRepo struct{ pool *pgxpool.Pool }

func NewUsersRepo(p *pgxpool.Pool) *UsersRepo { return &UsersRepo{pool: p} }

// UpsertFromWorkOS finds-or-creates a user keyed by workos_user_id.
// First user becomes admin; subsequent users default to non-admin.
// On re-upsert, last_login_at is bumped; is_admin is preserved.
func (r *UsersRepo) UpsertFromWorkOS(ctx context.Context, workosID, email string) (*User, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		// Rollback after a successful Commit returns ErrTxClosed — that's
		// expected and benign. Anything else is a real DB error worth
		// surfacing so we don't silently lose connection-level problems.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			log.Printf("users.UpsertFromWorkOS rollback: %v", rbErr)
		}
	}()

	var u User
	err = tx.QueryRow(ctx,
		`SELECT id, workos_user_id, email, is_admin, created_at, last_login_at
		 FROM users WHERE workos_user_id = $1`,
		workosID,
	).Scan(&u.ID, &u.WorkOSUserID, &u.Email, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt)

	switch {
	case err == nil:
		_, err = tx.Exec(ctx, `UPDATE users SET last_login_at = NOW(), email = $2 WHERE id = $1`, u.ID, email)
		if err != nil {
			return nil, err
		}
		u.Email = email
	case errors.Is(err, pgx.ErrNoRows):
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
			return nil, err
		}
		isAdmin := count == 0
		err = tx.QueryRow(ctx,
			`INSERT INTO users (workos_user_id, email, is_admin, last_login_at)
			 VALUES ($1, $2, $3, NOW())
			 RETURNING id, workos_user_id, email, is_admin, created_at, last_login_at`,
			workosID, email, isAdmin,
		).Scan(&u.ID, &u.WorkOSUserID, &u.Email, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt)
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UsersRepo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, workos_user_id, email, is_admin, created_at, last_login_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.WorkOSUserID, &u.Email, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns every user ordered by email. Intended for the admin users view.
func (r *UsersRepo) List(ctx context.Context) ([]User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, workos_user_id, email, is_admin, created_at, last_login_at
		 FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.WorkOSUserID, &u.Email, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
