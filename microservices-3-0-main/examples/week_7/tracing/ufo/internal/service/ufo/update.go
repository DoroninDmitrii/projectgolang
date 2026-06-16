package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/input"
)

func (s *service) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	err := s.ufoRepository.Update(ctx, uuid, updateInfo)
	if err != nil {
		if !errors.Is(err, errs.ErrSightingNotFound) {
			slog.ErrorContext(ctx, "не удалось обновить наблюдение", "uuid", uuid, "error", err)
		}
		return fmt.Errorf("обновить наблюдение: %w", err)
	}

	return nil
}
