package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/errors"
)

func (s *service) Delete(ctx context.Context, uuid string) error {
	err := s.ufoRepo.Delete(ctx, uuid)
	if err != nil {
		if !errors.Is(err, errs.ErrSightingNotFound) {
			slog.ErrorContext(ctx, "не удалось удалить наблюдение", "uuid", uuid, "error", err)
		}
		return fmt.Errorf("удалить наблюдение: %w", err)
	}

	// Инвалидируем кеш. При ошибке клиенты могут читать удалённые данные до истечения TTL
	// В production стоит добавить повторные попытки (retry), чтобы гарантировать удаление из кеша
	if cacheErr := s.cacheRepo.Delete(ctx, uuid); cacheErr != nil {
		slog.WarnContext(ctx, "не удалось инвалидировать кеш после удаления", "uuid", uuid, "error", cacheErr)
	}

	return nil
}
