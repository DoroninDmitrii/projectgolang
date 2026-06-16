package ufo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/service/input"
)

func (s *service) Create(ctx context.Context, in input.CreateSightingInput) (string, error) {
	sighting := model.Sighting{
		Uuid:            uuid.NewString(),
		ObservedAt:      in.ObservedAt,
		Location:        in.Location,
		Description:     in.Description,
		Color:           in.Color,
		Sound:           in.Sound,
		DurationSeconds: in.DurationSeconds,
		CreatedAt:       time.Now(),
	}

	if err := s.ufoRepo.Create(ctx, sighting); err != nil {
		slog.ErrorContext(ctx, "не удалось создать наблюдение", "error", err)
		return "", fmt.Errorf("создать наблюдение: %w", err)
	}

	// Прогреваем кеш. Ошибка не влияет на результат — при следующем запросе данные будут прочитаны из БД
	if cacheErr := s.cacheRepo.Set(ctx, sighting.Uuid, sighting, s.cacheTTL); cacheErr != nil {
		slog.WarnContext(ctx, "не удалось сохранить наблюдение в кеш", "uuid", sighting.Uuid, "error", cacheErr)
	}

	return sighting.Uuid, nil
}
