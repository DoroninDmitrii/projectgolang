package v1

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/client/grpc/weather/v1/converter"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/pkg/proto/weather/v1"
)

type client struct {
	grpcClient weatherv1.WeatherServiceClient
}

// New создаёт обёртку над gRPC клиентом WeatherService
func New(grpcClient weatherv1.WeatherServiceClient) *client {
	return &client{
		grpcClient: grpcClient,
	}
}

// GetCurrentWeather получает текущие погодные условия по местоположению
func (c *client) GetCurrentWeather(ctx context.Context, location string) (model.WeatherConditions, error) {
	resp, err := c.grpcClient.GetCurrentWeather(ctx, &weatherv1.GetCurrentWeatherRequest{
		Location: location,
	})
	if err != nil {
		// Маппинг gRPC ошибок в доменные ошибки
		if st, ok := status.FromError(err); ok {
			if st.Code() == codes.Unavailable {
				return model.WeatherConditions{}, errs.ErrWeatherUnavailable
			}
		}

		return model.WeatherConditions{}, fmt.Errorf("получить текущую погоду: %w", err)
	}

	return converter.WeatherConditionsFromProto(resp.GetConditions()), nil
}
