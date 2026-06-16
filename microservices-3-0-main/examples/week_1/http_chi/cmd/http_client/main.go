package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_chi/pkg/models"
)

const (
	serverURL         = "http://localhost:8080"
	weatherAPIPath    = "/api/v1/weather/%s"
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
	requestTimeout    = 5 * time.Second
	defaultCityName   = "Moscow"
	defaultMinTemp    = -10
	defaultMaxTemp    = 40
)

// httpClient - HTTP клиент с таймаутом
var httpClient = &http.Client{
	Timeout: requestTimeout,
}

// getWeather получает информацию о погоде для указанного города
func getWeather(ctx context.Context, city string) (*models.Weather, error) {
	// Создаем запрос с контекстом
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s"+weatherAPIPath, serverURL, city),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("создание GET-запроса: %w", err)
	}

	var resp *http.Response
	resp, err = httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение GET-запроса: %w", err)
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			slog.Error("ошибка закрытия тела ответа", "error", cerr)
			return
		}
	}()

	// Если статус ответа 404, считаем что данных нет
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	// Читаем тело ответа для любого ответа (для логирования при ошибке)
	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение тела ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("получение данных о погоде (статус %d): %s", resp.StatusCode, string(body))
	}

	var weather models.Weather
	err = json.Unmarshal(body, &weather)
	if err != nil {
		return nil, fmt.Errorf("декодирование JSON: %w", err)
	}

	return &weather, nil
}

// updateWeather обновляет данные о погоде для указанного города
func updateWeather(ctx context.Context, city string, weather *models.Weather) (*models.Weather, error) {
	// Кодируем данные о погоде в JSON
	jsonData, err := json.Marshal(weather)
	if err != nil {
		return nil, fmt.Errorf("кодирование JSON: %w", err)
	}

	// Создаем PUT-запрос с контекстом
	var req *http.Request
	req, err = http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fmt.Sprintf("%s"+weatherAPIPath, serverURL, city),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("создание PUT-запроса: %w", err)
	}
	req.Header.Set(contentTypeHeader, contentTypeJSON)

	// Выполняем запрос
	var resp *http.Response
	resp, err = httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение PUT-запроса: %w", err)
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			slog.Error("ошибка закрытия тела ответа", "error", cerr)
			return
		}
	}()

	// Читаем тело ответа
	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение тела ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("обновление данных о погоде (статус %d): %s", resp.StatusCode, string(body))
	}

	// Декодируем ответ
	var updatedWeather models.Weather
	err = json.Unmarshal(body, &updatedWeather)
	if err != nil {
		return nil, fmt.Errorf("декодирование JSON: %w", err)
	}

	return &updatedWeather, nil
}

// generateRandomWeather создает случайные данные о погоде
func generateRandomWeather() *models.Weather {
	return &models.Weather{
		Temperature: gofakeit.Float64Range(defaultMinTemp, defaultMaxTemp),
	}
}

func main() {
	ctx := context.Background()

	slog.Info("=== Тестирование API для работы с данными о погоде ===")
	slog.Info("")

	// 1. Пытаемся получить данные о погоде (которых пока нет)
	slog.Info("🌦️ получение данных о погоде для города", "city", defaultCityName)
	slog.Info("===================================================")

	weather, err := getWeather(ctx, defaultCityName)
	if err != nil {
		slog.Error("❌ ошибка", "error", err)
		return
	}

	slog.Info("данные о погоде для города", "city", defaultCityName, "weather", weather)

	// 2. Обновляем данные о погоде
	slog.Info("🔄 обновление данных о погоде для города", "city", defaultCityName)
	slog.Info("=====================================================")

	newWeather := generateRandomWeather()

	updatedWeather, err := updateWeather(ctx, defaultCityName, newWeather)
	if err != nil {
		slog.Error("❌ ошибка при обновлении погоды", "error", err)
		return
	}
	slog.Info("✅ данные о погоде обновлены", "weather", updatedWeather)

	// 3. Получаем обновленные данные о погоде
	slog.Info("🌦️ получение обновленных данных о погоде для города", "city", defaultCityName)
	slog.Info("===========================================================")

	weather, err = getWeather(ctx, defaultCityName)
	if err != nil {
		slog.Error("❌ ошибка при получении погоды", "error", err)
		return
	}

	if weather == nil {
		slog.Error("❌ неожиданно: данные о погоде отсутствуют после обновления")
		return
	}

	slog.Info("✅ получены данные о погоде", "weather", weather)
	slog.Info("тестирование завершено успешно!")
}
