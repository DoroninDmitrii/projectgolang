package v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/service/input"
)

// UFOService определяет контракт бизнес-логики для работы с наблюдениями НЛО
type UFOService interface {
	Create(ctx context.Context, in input.CreateSightingInput) (string, error)
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, in input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}
