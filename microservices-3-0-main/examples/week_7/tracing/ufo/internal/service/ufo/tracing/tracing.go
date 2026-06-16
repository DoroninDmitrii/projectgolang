package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/input"
)

const tracerName = "ufo"

// UFOService интерфейс бизнес-логики UFO сервиса
type UFOService interface {
	Create(ctx context.Context, in input.CreateSightingInput) (string, error)
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
	AnalyzeSighting(ctx context.Context, uuid string) (model.AnalysisResult, error)
}

type tracedService struct {
	inner UFOService
}

// NewTracedService создаёт трейсинг-декоратор для UFO сервиса
func NewTracedService(inner UFOService) UFOService {
	return &tracedService{inner: inner}
}

// Create оборачивает создание наблюдения в спан
func (t *tracedService) Create(ctx context.Context, in input.CreateSightingInput) (string, error) {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "ufo.create",
		trace.WithAttributes(
			attribute.String("sighting.description", in.Description),
		),
	)
	defer span.End()

	uuid, err := t.inner.Create(ctx, in)
	if err != nil {
		span.RecordError(err)
		return "", err
	}

	span.SetAttributes(attribute.String("ufo.uuid", uuid))

	return uuid, nil
}

// Get оборачивает получение наблюдения в спан
func (t *tracedService) Get(ctx context.Context, uuid string) (model.Sighting, error) {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "ufo.get",
		trace.WithAttributes(
			attribute.String("ufo.uuid", uuid),
		),
	)
	defer span.End()

	sighting, err := t.inner.Get(ctx, uuid)
	if err != nil {
		span.RecordError(err)
		return model.Sighting{}, err
	}

	return sighting, nil
}

// Update оборачивает обновление наблюдения в спан
func (t *tracedService) Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "ufo.update",
		trace.WithAttributes(
			attribute.String("ufo.uuid", uuid),
		),
	)
	defer span.End()

	err := t.inner.Update(ctx, uuid, updateInfo)
	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

// Delete оборачивает удаление наблюдения в спан
func (t *tracedService) Delete(ctx context.Context, uuid string) error {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "ufo.delete",
		trace.WithAttributes(
			attribute.String("ufo.uuid", uuid),
		),
	)
	defer span.End()

	err := t.inner.Delete(ctx, uuid)
	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

// AnalyzeSighting оборачивает анализ наблюдения в спан
func (t *tracedService) AnalyzeSighting(ctx context.Context, uuid string) (model.AnalysisResult, error) {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "ufo.analyze_sighting",
		trace.WithAttributes(
			attribute.String("ufo.uuid", uuid),
		),
	)
	defer span.End()

	result, err := t.inner.AnalyzeSighting(ctx, uuid)
	if err != nil {
		span.RecordError(err)
		return model.AnalysisResult{}, err
	}

	span.SetAttributes(
		attribute.String("analysis.classification", result.Classification),
		attribute.Float64("analysis.confidence", float64(result.ConfidenceScore)),
		attribute.String("analysis.result", result.AnalysisResult),
	)

	return result, nil
}
