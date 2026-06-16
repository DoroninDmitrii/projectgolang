package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/input"
)

// UFORepository интерфейс доступа к данным
type UFORepository interface {
	Create(ctx context.Context, sighting model.Sighting) error
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}

// AnalysisClient интерфейс для взаимодействия с Analysis сервисом
type AnalysisClient interface {
	AnalyzeSighting(ctx context.Context, uuid string) (model.AnalysisResult, error)
}
