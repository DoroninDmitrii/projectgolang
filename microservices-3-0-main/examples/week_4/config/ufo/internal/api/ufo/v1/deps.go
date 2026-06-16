package v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/service/input"
)

// UFOService — интерфейс сервисного слоя, от которого зависит API
type UFOService interface {
	Create(ctx context.Context, in input.CreateSightingInput) (string, error)
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, in input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}
