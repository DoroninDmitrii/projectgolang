package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/model"
)

func (s *service) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	sighting, err := s.ufoRepository.Get(ctx, uuid)
	if err != nil {
		if !errors.Is(err, errs.ErrSightingNotFound) {
			slog.ErrorContext(ctx, "не удалось получить наблюдение", "uuid", uuid, "error", err)
		}
		return model.Sighting{}, fmt.Errorf("получить наблюдение: %w", err)
	}

	return sighting, nil
}
