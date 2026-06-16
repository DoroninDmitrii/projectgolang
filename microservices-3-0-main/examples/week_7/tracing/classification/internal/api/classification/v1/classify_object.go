package classification_v1

import (
	"context"

	classificationv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/classification/v1"
)

// ClassifyObject обрабатывает запрос на классификацию объекта
func (a *api) ClassifyObject(ctx context.Context, req *classificationv1.ClassifyObjectRequest) (*classificationv1.ClassifyObjectResponse, error) {
	result, err := a.classificationService.ClassifyObject(
		ctx,
		req.GetDescription(),
		req.GetColor(),
		req.GetDurationSeconds(),
	)
	if err != nil {
		return nil, err
	}

	return &classificationv1.ClassifyObjectResponse{
		ObjectType:  result.ObjectType,
		Confidence:  result.Confidence,
		Explanation: result.Explanation,
	}, nil
}
