package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/errors"
)

func (s *service) Delete(ctx context.Context, uuid string) error {
	err := s.ufoRepo.Delete(ctx, uuid)
	if err != nil {
		if !errors.Is(err, errs.ErrSightingNotFound) {
			slog.ErrorContext(ctx, "не удалось удалить наблюдение", "uuid", uuid, "error", err)
		}
		return fmt.Errorf("удалить наблюдение: %w", err)
	}

	return nil
}
