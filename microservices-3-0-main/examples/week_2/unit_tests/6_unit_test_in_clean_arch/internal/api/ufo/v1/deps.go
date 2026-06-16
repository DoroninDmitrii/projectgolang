package v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/service/input"
)

type UFOService interface {
	Create(ctx context.Context, in input.CreateSightingInput) (string, error)
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, in input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}
