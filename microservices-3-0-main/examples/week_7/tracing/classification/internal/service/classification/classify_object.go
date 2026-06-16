package classification

import (
	"context"
	"strings"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/model"
)

// ClassifyObject классифицирует объект на основе описания, цвета и длительности
func (s *service) ClassifyObject(_ context.Context, description, color string, durationSeconds int32) (model.ClassificationResult, error) {
	result := classifyByShape(strings.ToLower(description))
	result = adjustByColor(result, strings.ToLower(color))
	result = adjustByDuration(result, durationSeconds)
	result.Confidence = min(max(result.Confidence, 0), 1)

	return result, nil
}
