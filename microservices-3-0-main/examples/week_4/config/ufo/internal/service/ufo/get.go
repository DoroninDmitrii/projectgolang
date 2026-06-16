package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/model"
)

func (s *service) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	return s.ufoRepository.Get(ctx, uuid)
}
