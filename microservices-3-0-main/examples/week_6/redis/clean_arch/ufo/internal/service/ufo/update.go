package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/service/input"
)

func (s *service) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	err := s.ufoRepo.Update(ctx, uuid, updateInfo)
	if err != nil {
		if !errors.Is(err, errs.ErrSightingNotFound) {
			slog.ErrorContext(ctx, "не удалось обновить наблюдение", "uuid", uuid, "error", err)
		}
		return fmt.Errorf("обновить наблюдение: %w", err)
	}

	// Инвалидируем кеш. При ошибке клиенты могут читать устаревшие данные до истечения TTL
	// В production стоит добавить повторные попытки (retry), чтобы гарантировать удаление из кеша
	if cacheErr := s.cacheRepo.Delete(ctx, uuid); cacheErr != nil {
		slog.WarnContext(ctx, "не удалось инвалидировать кеш после обновления", "uuid", uuid, "error", cacheErr)
	}

	return nil
}
