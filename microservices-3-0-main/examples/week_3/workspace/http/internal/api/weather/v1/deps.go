package v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/model"
)

// WeatherRepository — интерфейс репозитория для работы с данными о погоде
type WeatherRepository interface {
	Get(ctx context.Context, city string) (model.Weather, error)
	Upsert(ctx context.Context, city string, temperature float64) (model.Weather, error)
}
