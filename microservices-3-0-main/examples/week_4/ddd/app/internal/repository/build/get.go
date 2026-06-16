package build

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/record"
)

// Get возвращает сборку ПК по UUID
func (r *repository) Get(ctx context.Context, uuid string) (entity.PCBuild, error) {
	const query = `SELECT uuid, status, created_at, updated_at
		FROM pc_builds WHERE uuid = $1`

	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx, query, uuid)
	if err != nil {
		return entity.PCBuild{}, err
	}

	rec, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[record.BuildRecord])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.PCBuild{}, errs.ErrBuildNotFound
		}

		return entity.PCBuild{}, err
	}

	return repoConverter.BuildRecordToModel(rec)
}
