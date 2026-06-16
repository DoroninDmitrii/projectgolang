// Package tracing реализует декоратор для обогащения трейсов бизнес-атрибутами
//
// Декоратор оборачивает весь вызов метода сервиса в кастомный спан
// Спаны на отдельные внешние вызовы (gRPC, SQL и т.д.) создавать вручную не нужно —
// их автоматически создают инструментации (otelgrpc, otelsql и др.), подключённые
// на уровне транспорта (см. grpc.WithStatsHandler(otelgrpc.NewClientHandler()) в main.go)
//
// Если нужно обернуть спаном не весь метод, а его часть (например, CPU-bound вычисления
// без внешних вызовов), можно создать спан inline прямо в бизнес-логике:
//
//	ctx, span := otel.Tracer("analysis").Start(ctx, "analysis.heavy_computation")
//	defer span.End()
//
// Однако это засоряет бизнес-логику инфраструктурным кодом, поэтому предпочтительнее
// выделить тяжёлую часть в отдельный метод и обернуть его декоратором
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/model"
)

const tracerName = "analysis"

// AnalysisService — интерфейс сервиса анализа
type AnalysisService interface {
	AnalyzeSighting(ctx context.Context, uuid string) (model.ClassificationResult, error)
}

type tracedService struct {
	inner AnalysisService
}

// NewTracedService создаёт трейсинг-декоратор для сервиса анализа
func NewTracedService(inner AnalysisService) AnalysisService {
	return &tracedService{inner: inner}
}

// AnalyzeSighting оборачивает анализ наблюдения в спан с атрибутами
func (t *tracedService) AnalyzeSighting(ctx context.Context, uuid string) (model.ClassificationResult, error) {
	ctx, span := otel.Tracer(tracerName).Start(
		ctx, "analysis.analyze_sighting",
		trace.WithAttributes(
			attribute.String("ufo.uuid", uuid),
		),
	)
	defer span.End()

	result, err := t.inner.AnalyzeSighting(ctx, uuid)
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
