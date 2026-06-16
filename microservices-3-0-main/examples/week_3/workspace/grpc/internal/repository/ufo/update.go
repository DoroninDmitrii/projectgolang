package ufo

import (
	"context"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/service/input"
)

// Update обновляет наблюдение. COALESCE оставляет старое значение, если передан NULL
func (r *repository) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	query := `UPDATE sightings SET
		observed_at = COALESCE($1, observed_at),
		location = COALESCE($2, location),
		description = COALESCE($3, description),
		color = COALESCE($4, color),
		sound = COALESCE($5, sound),
		duration_seconds = COALESCE($6, duration_seconds),
		updated_at = now()
		WHERE uuid = $7 AND deleted_at IS NULL`

	result, err := r.pool.Exec(
		ctx, query,
		updateInfo.ObservedAt,
		updateInfo.Location,
		updateInfo.Description,
		updateInfo.Color,
		updateInfo.Sound,
		updateInfo.DurationSeconds,
		uuid,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return errs.ErrSightingNotFound
	}

	return nil
}
