package ufo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/service/input"
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

	return sighting.Uuid, nil
}
