package ufo

import (
	"context"
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/errors"
)

func (r *repository) Delete(ctx context.Context, uuid string) error {
	query := `UPDATE sightings SET deleted_at = $1 WHERE uuid = $2 AND deleted_at IS NULL`

	res, err := r.pool.Exec(ctx, query, time.Now(), uuid)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return errs.ErrSightingNotFound
	}

	return nil
}
