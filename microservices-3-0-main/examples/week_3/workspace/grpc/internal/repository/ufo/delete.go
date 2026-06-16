package ufo

import (
	"context"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/errors"
)

func (r *repository) Delete(ctx context.Context, uuid string) error {
	query := `UPDATE sightings SET deleted_at = now() WHERE uuid = $1 AND deleted_at IS NULL`

	result, err := r.pool.Exec(ctx, query, uuid)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return errs.ErrSightingNotFound
	}

	return nil
}
