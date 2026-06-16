package ufo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/converter"
	repoModel "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/model"
)

func (r *repository) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	query := `SELECT uuid, observed_at, location, description, color, sound, duration_seconds, created_at, updated_at, deleted_at
		FROM sightings WHERE uuid = $1 AND deleted_at IS NULL`

	rows, err := r.pool.Query(ctx, query, uuid)
	if err != nil {
		return model.Sighting{}, err
	}

	// pgx.CollectOneRow собирает ровно одну строку и закрывает rows
	// pgx.RowToStructByName автоматически маппит колонки в поля структуры по тегам `db:"..."`.
	pgView, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[repoModel.SightingPGView])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Sighting{}, errs.ErrSightingNotFound
		}

		return model.Sighting{}, err
	}

	return repoConverter.SightingFromPGView(pgView), nil
}
