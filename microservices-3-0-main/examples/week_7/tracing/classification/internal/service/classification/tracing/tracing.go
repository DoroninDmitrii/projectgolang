package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/model"
)

const tracerName = "classification"

// ClassificationService — интерфейс сервиса классификации
type ClassificationService interface {
	ClassifyObject(ctx context.Context, description, color string, durationSeconds int32) (model.ClassificationResult, error)
}

type tracedService struct {
	inner ClassificationService
}

// NewTracedService создаёт трейсинг-декоратор для сервиса классификации
func NewTracedService(inner ClassificationService) ClassificationService {
	return &tracedService{inner: inner}
}

// ClassifyObject оборачивает вызов классификации в спан с атрибутами
func (t *tracedService) ClassifyObject(ctx context.Context, description, color string, durationSeconds int32) (model.ClassificationResult, error) {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "classification.classify_object",
		trace.WithAttributes(
			attribute.String("sighting.description", description),
			attribute.String("sighting.color", color),
			attribute.Int("sighting.duration_seconds", int(durationSeconds)),
		),
	)
	defer span.End()

	result, err := t.inner.ClassifyObject(ctx, description, color, durationSeconds)
	if err != nil {
		span.RecordError(err)
		return model.ClassificationResult{}, err
	}

	span.SetAttributes(
		attribute.String("classification.object_type", result.ObjectType),
		attribute.Float64("classification.confidence", float64(result.Confidence)),
		attribute.String("classification.explanation", result.Explanation),
	)

	return result, nil
}
