package build

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// Create создаёт запись о сборке ПК. UUID и статус приходят сверху из сервисного слоя,
// БД ничего не генерит и не дефолтит.
func (r *repository) Create(ctx context.Context, buildUUID string, status valueobject.BuildStatus) error {
	const query = `INSERT INTO pc_builds (uuid, status) VALUES ($1, $2)`

	if _, err := r.getter.DefaultTrOrDB(ctx, r.pool).Exec(ctx, query, buildUUID, status); err != nil {
		return err
	}

	return nil
}
