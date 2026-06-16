package v1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/ufo/v1"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/errors"
)

// AnalyzeSighting анализирует наблюдение НЛО
func (a *api) AnalyzeSighting(ctx context.Context, req *ufov1.AnalyzeSightingRequest) (*ufov1.AnalyzeSightingResponse, error) {
	result, err := a.ufoService.AnalyzeSighting(ctx, req.Uuid)
	if err != nil {
		// errors.Is — ловит доменную ошибку из локального репозитория
		// status.Code — ловит gRPC NotFound от внешнего analysis-сервиса,
		// который клиентский слой пробрасывает как есть
		if errors.Is(err, errs.ErrSightingNotFound) || status.Code(err) == codes.NotFound {
			return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.Uuid)
		}
		return nil, status.Errorf(codes.Internal, "не удалось проанализировать наблюдение: %v", err)
	}

	return &ufov1.AnalyzeSightingResponse{
		AnalysisResult:  result.AnalysisResult,
		Classification:  result.Classification,
		ConfidenceScore: result.ConfidenceScore,
	}, nil
}
