package ufo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/model"
)

func (r *repository) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	query := `SELECT uuid, observed_at, location, description, color, sound, duration_seconds, created_at, updated_at, deleted_at
		FROM sightings WHERE uuid = $1`

	var s model.Sighting
	err := r.pool.QueryRow(ctx, query, uuid).Scan(
		&s.Uuid, &s.ObservedAt, &s.Location, &s.Description,
		&s.Color, &s.Sound, &s.DurationSeconds,
		&s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Sighting{}, errs.ErrSightingNotFound
		}
		return model.Sighting{}, err
	}

	return s, nil
}
