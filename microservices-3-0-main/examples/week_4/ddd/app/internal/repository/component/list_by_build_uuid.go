package component

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/record"
)

// ListByBuildUUID возвращает компоненты, привязанные к сборке
func (r *repository) ListByBuildUUID(ctx context.Context, buildUUID string) ([]entity.Component, error) {
	const query = `
		SELECT c.uuid, c.name, c.type, c.properties, c.stock_quantity, c.reserved, c.created_at, c.updated_at
		FROM components c
		JOIN pc_build_components bc ON bc.component_uuid = c.uuid
		WHERE bc.build_uuid = $1
	`

	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx, query, buildUUID)
	if err != nil {
		return nil, fmt.Errorf("получить компоненты сборки: %w", err)
	}

	records, err := pgx.CollectRows(rows, pgx.RowToStructByName[record.ComponentRecord])
	if err != nil {
		return nil, fmt.Errorf("считать строки: %w", err)
	}

	return repoConverter.ComponentRecordsToModels(records)
}
