package analysis_v1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analysisv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/analysis/v1"
)

// AnalyzeSighting обрабатывает запрос на анализ наблюдения
func (a *api) AnalyzeSighting(ctx context.Context, req *analysisv1.AnalyzeSightingRequest) (*analysisv1.AnalyzeSightingResponse, error) {
	result, err := a.analysisService.AnalyzeSighting(ctx, req.GetUuid())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "не удалось проанализировать наблюдение: %v", err)
	}

	return &analysisv1.AnalyzeSightingResponse{
		AnalysisResult:  result.AnalysisResult,
		Classification:  result.ObjectType,
		ConfidenceScore: result.Confidence,
	}, nil
}
