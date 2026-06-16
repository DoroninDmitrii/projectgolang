package ufo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/service/input"
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
	sighting.Credibility = calculateCredibility(sighting)

	if err := s.ufoRepository.Create(ctx, sighting); err != nil {
		return "", fmt.Errorf("сохранить наблюдение: %w", err)
	}

	return sighting.Uuid, nil
}
