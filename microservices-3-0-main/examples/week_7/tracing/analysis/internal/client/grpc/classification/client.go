package classification

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/model"
	classificationv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/classification/v1"
)

type client struct {
	client classificationv1.ClassificationServiceClient
}

// New создаёт новый клиент для Classification сервиса
func New(grpcClient classificationv1.ClassificationServiceClient) *client {
	return &client{
		client: grpcClient,
	}
}

// ClassifyObject классифицирует объект на основе описания, цвета и длительности
func (c *client) ClassifyObject(ctx context.Context, description, color string, durationSeconds int32) (model.ClassificationResult, error) {
	resp, err := c.client.ClassifyObject(ctx, &classificationv1.ClassifyObjectRequest{
		Description:     description,
		Color:           color,
		DurationSeconds: durationSeconds,
	})
	if err != nil {
		return model.ClassificationResult{}, err
	}

	return model.ClassificationResult{
		ObjectType:  resp.ObjectType,
		Confidence:  resp.Confidence,
		Explanation: resp.Explanation,
	}, nil
}
