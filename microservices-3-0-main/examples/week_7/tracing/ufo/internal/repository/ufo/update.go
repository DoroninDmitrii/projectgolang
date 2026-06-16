package ufo

import (
	"context"
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/input"
)

func (r *repository) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	query := `
		UPDATE sightings SET
			updated_at       = $1,
			observed_at      = COALESCE($2, observed_at),
			location         = COALESCE($3, location),
			description      = COALESCE($4, description),
			color            = COALESCE($5, color),
			sound            = COALESCE($6, sound),
			duration_seconds = COALESCE($7, duration_seconds)
		WHERE uuid = $8 AND deleted_at IS NULL`

	res, err := r.pool.Exec(
		ctx, query,
		time.Now(),
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

	if res.RowsAffected() == 0 {
		return errs.ErrSightingNotFound
	}

	return nil
}
