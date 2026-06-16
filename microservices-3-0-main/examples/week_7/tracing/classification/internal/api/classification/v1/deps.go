package classification_v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/model"
)

// ClassificationService — интерфейс сервиса классификации
type ClassificationService interface {
	ClassifyObject(ctx context.Context, description, color string, durationSeconds int32) (model.ClassificationResult, error)
}
