package ufo

import (
	"context"
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/service/input"
)

func (r *repository) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	// COALESCE(новое, текущее) возвращает первый не-NULL аргумент
	// PostgreSQL при UPDATE уже загружает строку в память, поэтому текущие значения колонок доступны
	// без дополнительного чтения. Если параметр не NULL — колонка перезаписывается новым значением
	// Если NULL (nil в Go) — COALESCE возвращает текущее значение колонки, т.е. ничего не меняется
	// Это позволяет делать частичное обновление одним запросом без динамической сборки SQL
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
