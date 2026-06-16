package ufo_cache

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/converter"
	repoModel "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/model"
)

func (r *repository) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	cacheKey := r.getCacheKey(uuid)

	var sightingRedisView repoModel.SightingRedisView
	err := r.client.HGetAll(ctx, cacheKey).Scan(&sightingRedisView)
	if err != nil {
		// redis.Nil от HGetAll не приходит — эту ошибку возвращают только
		// строковые команды (GET, HGET и т.п.) для отсутствующего ключа.
		// Здесь ветка нужна только на случай, если кто-то прокинет Nil
		// сверху (например, обёрткой клиента). Реальный «not found»
		// у HGetAll ловится проверкой на пустой UUID ниже.
		if errors.Is(err, redis.Nil) {
			return model.Sighting{}, errs.ErrSightingNotFound
		}

		return model.Sighting{}, err
	}

	// HGetAll для несуществующего ключа возвращает пустую map БЕЗ ошибки.
	// Это единственный способ отличить «нет такого ключа» от «ключ есть, но пустой» —
	// проверяем обязательное поле первичного ключа в нашей view.
	if sightingRedisView.UUID == "" {
		return model.Sighting{}, errs.ErrSightingNotFound
	}

	return repoConverter.SightingFromRedisView(sightingRedisView), nil
}
