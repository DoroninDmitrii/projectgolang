package ufo

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/service/input"
)

func (r *repository) Create(ctx context.Context, in input.CreateSightingInput) (string, error) {
	id := uuid.New().String()
	now := time.Now()

	query := `INSERT INTO sightings (uuid, observed_at, location, description, color, sound, duration_seconds, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(
		ctx, query,
		id, in.ObservedAt, in.Location, in.Description,
		in.Color, in.Sound, in.DurationSeconds, now,
	)
	if err != nil {
		return "", err
	}

	return id, nil
}
