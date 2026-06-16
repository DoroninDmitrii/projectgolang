package component

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/record"
)

// List возвращает компоненты по списку UUID
// TODO: в production-коде необходимо добавить FOR UPDATE ORDER BY uuid для предотвращения deadlock'ов
func (r *repository) List(ctx context.Context, uuids []string) ([]entity.Component, error) {
	const query = `SELECT uuid, name, type, properties, stock_quantity, reserved, created_at, updated_at
		FROM components WHERE uuid = ANY($1)`

	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx, query, uuids)
	if err != nil {
		return nil, fmt.Errorf("получить компоненты: %w", err)
	}

	records, err := pgx.CollectRows(rows, pgx.RowToStructByName[record.ComponentRecord])
	if err != nil {
		return nil, fmt.Errorf("считать строки: %w", err)
	}

	return repoConverter.ComponentRecordsToModels(records)
}
