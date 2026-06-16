package ufo

import (
	"context"
	"fmt"
	"time"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/service/input"
)

func (s *service) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	sighting, err := s.ufoRepository.Get(ctx, uuid)
	if err != nil {
		return fmt.Errorf("получить наблюдение: %w", err)
	}

	applyUpdate(&sighting, updateInfo)

	sighting.Credibility = calculateCredibility(sighting)
	sighting.UpdatedAt = new(time.Now())

	if err = s.ufoRepository.Update(ctx, sighting); err != nil {
		return fmt.Errorf("обновить наблюдение: %w", err)
	}

	return nil
}

func applyUpdate(s *model.Sighting, updateInfo input.UpdateSightingInput) {
	if updateInfo.ObservedAt != nil {
		s.ObservedAt = updateInfo.ObservedAt
	}

	if updateInfo.Location != nil {
		s.Location = *updateInfo.Location
	}

	if updateInfo.Description != nil {
		s.Description = *updateInfo.Description
	}

	if updateInfo.Color != nil {
		s.Color = updateInfo.Color
	}

	if updateInfo.Sound != nil {
		s.Sound = updateInfo.Sound
	}

	if updateInfo.DurationSeconds != nil {
		s.DurationSeconds = updateInfo.DurationSeconds
	}
}
