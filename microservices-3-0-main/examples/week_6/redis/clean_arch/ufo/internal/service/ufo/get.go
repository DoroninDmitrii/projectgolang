package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
)

func (s *service) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	// Сначала пытаемся получить из кеша
	sighting, err := s.cacheRepo.Get(ctx, uuid)
	if nil == err {
		return sighting, nil
	}

	// Если нет в кеше или ошибка, идём в PostgreSQL
	sighting, err = s.ufoRepo.Get(ctx, uuid)
	if err != nil {
		if !errors.Is(err, errs.ErrSightingNotFound) {
			slog.ErrorContext(ctx, "не удалось получить наблюдение", "uuid", uuid, "error", err)
		}
		return model.Sighting{}, fmt.Errorf("получить наблюдение: %w", err)
	}

	// Прогреваем кеш. Ошибка не влияет на результат — при следующем запросе данные снова будут прочитаны из БД
	if cacheErr := s.cacheRepo.Set(ctx, uuid, sighting, s.cacheTTL); cacheErr != nil {
		slog.WarnContext(ctx, "не удалось сохранить наблюдение в кеш", "uuid", uuid, "error", cacheErr)
	}

	return sighting, nil
}
