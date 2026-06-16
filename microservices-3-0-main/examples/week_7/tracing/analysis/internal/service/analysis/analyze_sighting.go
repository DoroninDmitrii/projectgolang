package analysis

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/model"
)

// AnalyzeSighting анализирует наблюдение НЛО
func (s *service) AnalyzeSighting(ctx context.Context, uuid string) (model.ClassificationResult, error) {
	// Получаем данные о наблюдении из UFO сервиса
	sighting, err := s.ufoClient.GetSighting(ctx, uuid)
	if err != nil {
		return model.ClassificationResult{}, fmt.Errorf("не удалось получить наблюдение: %w", err)
	}

	// Отправляем данные в Classification сервис
	classification, err := s.classificationClient.ClassifyObject(ctx, sighting.Description, sighting.Color, sighting.DurationSeconds)
	if err != nil {
		return model.ClassificationResult{}, fmt.Errorf("не удалось классифицировать объект: %w", err)
	}

	classification.AnalysisResult = fmt.Sprintf(
		"Анализ наблюдения %s: Объект классифицирован как %s с уверенностью %.2f. %s",
		uuid,
		classification.ObjectType,
		classification.Confidence,
		classification.Explanation,
	)

	return classification, nil
}
