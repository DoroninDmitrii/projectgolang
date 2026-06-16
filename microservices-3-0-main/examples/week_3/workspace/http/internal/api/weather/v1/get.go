package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/api/converter"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/errors"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/openapi/weather/v1"
)

func (a *api) GetWeatherByCity(ctx context.Context, params weatherv1.GetWeatherByCityParams) (weatherv1.GetWeatherByCityRes, error) {
	weather, err := a.weatherRepository.Get(ctx, params.City)
	if err != nil {
		if errors.Is(err, errs.ErrWeatherNotFound) {
			return &weatherv1.GetWeatherByCityNotFound{
				Code:    http.StatusNotFound,
				Message: "данные о погоде для города '" + params.City + "' не найдены",
			}, nil
		}
		return nil, err
	}

	return converter.WeatherToDTO(weather), nil
}
