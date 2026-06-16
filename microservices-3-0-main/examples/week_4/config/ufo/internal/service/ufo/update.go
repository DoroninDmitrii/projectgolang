package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/service/input"
)

func (s *service) Update(ctx context.Context, uuid string, in input.UpdateSightingInput) error {
	return s.ufoRepository.Update(ctx, uuid, in)
}
