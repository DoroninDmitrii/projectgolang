package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/brianvoe/gofakeit/v7"

	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_chi_ogen/pkg/openapi/weather/v1"
)

const (
	serverURL       = "http://localhost:8080"
	defaultCityName = "Moscow"
	defaultMinTemp  = -10.0
	defaultMaxTemp  = 40.0
)

func main() {
	ctx := context.Background()

	// Инициализация Ogen-клиента
	client, err := weatherv1.NewClient(serverURL)
	if err != nil {
		slog.Error("ошибка при создании клиента", "error", err)
		os.Exit(1)
	}

	slog.Info("=== Тестирование API для работы с данными о погоде ===")

	// 1. Пытаемся получить данные о погоде (которых пока нет)
	slog.Info("получение данных о погоде для города", "city", defaultCityName)

	weatherResp, err := client.GetWeatherByCity(ctx, weatherv1.GetWeatherByCityParams{
		City: defaultCityName,
	})
	if err != nil {
		slog.Error("ошибка при получении погоды", "error", err)
		return
	}

	// Проверяем тип ответа - может быть Weather или ошибка (NotFound, BadRequest, etc.)
	switch resp := weatherResp.(type) {
	case *weatherv1.Weather:
		slog.Info("данные о погоде получены", "city", resp.City, "temperature", resp.Temperature)
	case *weatherv1.GetWeatherByCityNotFound:
		slog.Info("данные о погоде для города не найдены, создаём...", "city", defaultCityName)
	case *weatherv1.GetWeatherByCityBadRequest:
		slog.Error("некорректный запрос", "code", resp.Code, "message", resp.Message)
		return
	default:
		slog.Error("неизвестный тип ответа", "type", resp)
		return
	}

	// 2. Обновляем данные о погоде
	slog.Info("обновление данных о погоде для города", "city", defaultCityName)

	// Создаем запрос на обновление погоды
	updateRequest := &weatherv1.UpdateWeatherRequest{
		Temperature: gofakeit.Float64Range(defaultMinTemp, defaultMaxTemp),
	}

	updatedWeather, err := client.UpdateWeatherByCity(ctx, updateRequest, weatherv1.UpdateWeatherByCityParams{
		City: defaultCityName,
	})
	if err != nil {
		slog.Error("ошибка при обновлении погоды", "error", err)
		return
	}

	switch resp := updatedWeather.(type) {
	case *weatherv1.Weather:
		slog.Info("данные о погоде обновлены", "city", resp.City, "temperature", resp.Temperature)
	case *weatherv1.UpdateWeatherByCityBadRequest:
		slog.Error("некорректный запрос при обновлении", "code", resp.Code, "message", resp.Message)
		return
	default:
		slog.Error("неизвестный тип ответа при обновлении", "type", resp)
		return
	}

	// 3. Получаем обновленные данные о погоде
	slog.Info("получение обновленных данных о погоде для города", "city", defaultCityName)

	weatherResp, err = client.GetWeatherByCity(ctx, weatherv1.GetWeatherByCityParams{
		City: defaultCityName,
	})
	if err != nil {
		slog.Error("ошибка при получении погоды", "error", err)
		return
	}

	if weather, ok := weatherResp.(*weatherv1.Weather); ok {
		slog.Info(
			"получены данные о погоде",
			"city", weather.City,
			"temperature", weather.Temperature,
			"updated_at", weather.UpdatedAt,
		)
	}

	slog.Info("тестирование завершено успешно!")
}
