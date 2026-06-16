package ufo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/service/input"
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

	if err := s.ufoRepository.Create(ctx, sighting); err != nil {
		return "", fmt.Errorf("сохранить наблюдение: %w", err)
	}

	return sighting.Uuid, nil
}
