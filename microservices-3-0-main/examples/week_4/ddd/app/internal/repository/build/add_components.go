package build

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

// AddComponents создаёт связи между сборкой и компонентами одним multi-insert запросом.
// UUID связующих строк генерится в Go, БД ничего не дефолтит.
func (r *repository) AddComponents(ctx context.Context, buildUUID string, componentUUIDs []string) error {
	builder := sq.Insert("pc_build_components").
		Columns("uuid", "build_uuid", "component_uuid").
		PlaceholderFormat(sq.Dollar)

	for _, componentUUID := range componentUUIDs {
		builder = builder.Values(uuid.NewString(), buildUUID, componentUUID)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("собрать запрос: %w", err)
	}

	_, err = r.getter.DefaultTrOrDB(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("добавить компоненты к сборке: %w", err)
	}

	return nil
}
