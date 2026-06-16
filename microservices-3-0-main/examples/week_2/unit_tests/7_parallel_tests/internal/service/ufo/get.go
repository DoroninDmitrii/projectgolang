package ufo

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/model"
)

func (s *service) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	// В реальном приложении здесь может быть бизнес-логика:
	// проверка прав доступа, фильтрация полей, кэширование и т.д

	sighting, err := s.ufoRepository.Get(ctx, uuid)
	if err != nil {
		return model.Sighting{}, fmt.Errorf("получить наблюдение: %w", err)
	}

	return sighting, nil
}
