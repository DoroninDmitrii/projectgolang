package v1

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/api/converter"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/openapi/weather/v1"
)

func (a *api) UpdateWeatherByCity(ctx context.Context, req *weatherv1.UpdateWeatherRequest, params weatherv1.UpdateWeatherByCityParams) (weatherv1.UpdateWeatherByCityRes, error) {
	weather, err := a.weatherRepository.Upsert(ctx, params.City, req.Temperature)
	if err != nil {
		return nil, err
	}

	return converter.WeatherToDTO(weather), nil
}
