package analysis

import (
	"context"

	analysisv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/analysis/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/model"
)

type client struct {
	grpcClient analysisv1.AnalysisServiceClient
}

// New создает новый клиент для Analysis сервиса
func New(grpcClient analysisv1.AnalysisServiceClient) *client {
	return &client{
		grpcClient: grpcClient,
	}
}

// AnalyzeSighting анализирует наблюдение НЛО
func (c *client) AnalyzeSighting(ctx context.Context, uuid string) (model.AnalysisResult, error) {
	resp, err := c.grpcClient.AnalyzeSighting(ctx, &analysisv1.AnalyzeSightingRequest{
		Uuid: uuid,
	})
	if err != nil {
		return model.AnalysisResult{}, err
	}

	return model.AnalysisResult{
		AnalysisResult:  resp.AnalysisResult,
		Classification:  resp.Classification,
		ConfidenceScore: resp.ConfidenceScore,
	}, nil
}
