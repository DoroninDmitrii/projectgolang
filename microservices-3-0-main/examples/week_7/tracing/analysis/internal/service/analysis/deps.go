package analysis

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/model"
)

// UFOClient — интерфейс клиента UFO сервиса
type UFOClient interface {
	GetSighting(ctx context.Context, uuid string) (model.Sighting, error)
}

// ClassificationClient — интерфейс клиента Classification сервиса
type ClassificationClient interface {
	ClassifyObject(ctx context.Context, description, color string, durationSeconds int32) (model.ClassificationResult, error)
}
