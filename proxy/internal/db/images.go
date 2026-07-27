package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImagesRepo struct{ pool *pgxpool.Pool }

func NewImagesRepo(p *pgxpool.Pool) *ImagesRepo { return &ImagesRepo{pool: p} }

type ApplyRecord struct {
	ID          uuid.UUID
	RoverID     string
	URL         string
	Version     *string
	RequestedBy uuid.UUID
	RequestedAt time.Time
	Status      string
}

// RecordApply inserts a new image_apply_history row and returns the full record.
// version is *string so an unset value writes SQL NULL.
func (r *ImagesRepo) RecordApply(ctx context.Context, roverID, url string, version *string, user uuid.UUID) (*ApplyRecord, error) {
	var rec ApplyRecord
	err := r.pool.QueryRow(ctx,
		`INSERT INTO image_apply_history (rover_id, url, version, requested_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, rover_id, url, version, requested_by, requested_at, status`,
		roverID, url, version, user,
	).Scan(&rec.ID, &rec.RoverID, &rec.URL, &rec.Version, &rec.RequestedBy, &rec.RequestedAt, &rec.Status)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateStatus sets the status column for an existing apply history row.
func (r *ImagesRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE image_apply_history SET status = $2 WHERE id = $1`, id, status)
	return err
}
