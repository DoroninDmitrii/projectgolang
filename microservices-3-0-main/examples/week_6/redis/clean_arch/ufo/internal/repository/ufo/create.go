package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
)

func (r *repository) Create(ctx context.Context, s model.Sighting) error {
	query := `INSERT INTO sightings (uuid, observed_at, location, description, color, sound, duration_seconds, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(
		ctx, query,
		s.Uuid, s.ObservedAt, s.Location, s.Description,
		s.Color, s.Sound, s.DurationSeconds, s.CreatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}
