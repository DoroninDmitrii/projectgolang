package component

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/record"
)

// Get возвращает компонент по UUID
func (r *repository) Get(ctx context.Context, uuid string) (entity.Component, error) {
	const query = `SELECT uuid, name, type, properties, stock_quantity, reserved, created_at, updated_at
		FROM components WHERE uuid = $1`

	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx, query, uuid)
	if err != nil {
		return entity.Component{}, fmt.Errorf("получить компонент: %w", err)
	}

	rec, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[record.ComponentRecord])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Component{}, errs.ErrComponentNotFound
		}

		return entity.Component{}, fmt.Errorf("получить компонент: %w", err)
	}

	return repoConverter.ComponentRecordToModel(&rec)
}
