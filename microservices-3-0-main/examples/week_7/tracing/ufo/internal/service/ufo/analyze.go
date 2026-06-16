package ufo

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/model"
)

// AnalyzeSighting анализирует наблюдение НЛО через Analysis сервис
func (s *service) AnalyzeSighting(ctx context.Context, uuid string) (model.AnalysisResult, error) {
	result, err := s.analysisClient.AnalyzeSighting(ctx, uuid)
	if err != nil {
		slog.ErrorContext(ctx, "не удалось проанализировать наблюдение", "uuid", uuid, "error", err)
		return model.AnalysisResult{}, fmt.Errorf("проанализировать наблюдение: %w", err)
	}

	return result, nil
}
