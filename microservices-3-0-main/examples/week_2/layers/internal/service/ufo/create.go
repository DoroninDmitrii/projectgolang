package ufo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/service/input"
)

func (s *service) Create(ctx context.Context, in input.CreateSightingInput) (string, error) {
	// Запрашиваем погоду по местоположению наблюдения
	// Если сервис погоды недоступен — логируем и продолжаем (graceful degradation)
	weather, err := s.weatherClient.GetCurrentWeather(ctx, in.Location)
	var weatherPtr *model.WeatherConditions
	if err != nil {
		if errors.Is(err, errs.ErrWeatherUnavailable) {
			slog.WarnContext(ctx, "сервис погоды недоступен, продолжаем без данных о погоде",
				"location", in.Location)
		} else {
			return "", fmt.Errorf("получить погоду: %w", err)
		}
	} else {
		weatherPtr = &weather
	}

	sighting := model.Sighting{
		Uuid:            uuid.NewString(),
		ObservedAt:      in.ObservedAt,
		Location:        in.Location,
		Description:     in.Description,
		Color:           in.Color,
		Sound:           in.Sound,
		DurationSeconds: in.DurationSeconds,
		Weather:         weatherPtr,
		CreatedAt:       time.Now(),
	}
	sighting.Credibility = calculateCredibility(sighting)

	if err = s.ufoRepository.Create(ctx, sighting); err != nil {
		return "", fmt.Errorf("сохранить наблюдение: %w", err)
	}

	return sighting.Uuid, nil
}
