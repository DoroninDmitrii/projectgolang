package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/service/input"
)

// UFORepository — интерфейс репозитория, от которого зависит сервис
type UFORepository interface {
	Create(ctx context.Context, sighting model.Sighting) error
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, in input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}
