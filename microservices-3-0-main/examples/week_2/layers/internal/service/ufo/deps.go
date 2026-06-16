package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
)

type UFORepository interface {
	Create(ctx context.Context, sighting model.Sighting) error
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, sighting model.Sighting) error
	Delete(ctx context.Context, uuid string) error
}

type WeatherClient interface {
	GetCurrentWeather(ctx context.Context, location string) (model.WeatherConditions, error)
}
