package build

import (
	"context"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// UpdateStatus обновляет статус сборки ПК
func (r *repository) UpdateStatus(ctx context.Context, uuid string, status valueobject.BuildStatus) error {
	const query = `UPDATE pc_builds SET status = $1, updated_at = NOW() WHERE uuid = $2`

	tag, err := r.getter.DefaultTrOrDB(ctx, r.pool).Exec(ctx, query, status, uuid)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrBuildNotFound
	}

	return nil
}
