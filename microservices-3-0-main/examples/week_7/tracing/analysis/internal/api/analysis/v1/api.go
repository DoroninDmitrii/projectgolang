package analysis_v1

import (
	analysisv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/analysis/v1"
)

// api — gRPC хендлер для сервиса анализа
type api struct {
	analysisv1.UnimplementedAnalysisServiceServer
	analysisService AnalysisService
}

// NewAPI создаёт новый API анализа
func NewAPI(analysisService AnalysisService) *api {
	return &api{
		analysisService: analysisService,
	}
}
