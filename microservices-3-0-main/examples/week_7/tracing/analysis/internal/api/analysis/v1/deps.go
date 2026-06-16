package analysis_v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/model"
)

// AnalysisService — интерфейс сервиса анализа
type AnalysisService interface {
	AnalyzeSighting(ctx context.Context, uuid string) (model.ClassificationResult, error)
}
