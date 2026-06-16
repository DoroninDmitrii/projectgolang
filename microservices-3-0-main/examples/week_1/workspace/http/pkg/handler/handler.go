package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	customMiddleware "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/http/internal/middleware"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/shared/pkg/openapi/weather/v1"
)

const (
	middlewareTimeout = 10 * time.Second
)

// WeatherStorage представляет потокобезопасное хранилище данных о погоде
type WeatherStorage struct {
	mu       sync.RWMutex
	weathers map[string]*weatherv1.Weather
}

// NewWeatherStorage создает новое хранилище данных о погоде
func NewWeatherStorage() *WeatherStorage {
	return &WeatherStorage{
		weathers: make(map[string]*weatherv1.Weather),
	}
}

// GetWeather возвращает информацию о погоде по имени города
func (s *WeatherStorage) GetWeather(city string) *weatherv1.Weather {
	s.mu.RLock()
	defer s.mu.RUnlock()

	weather, ok := s.weathers[city]
	if !ok {
		return nil
	}

	return weather
}

// UpdateWeather обновляет данные о погоде для указанного города
func (s *WeatherStorage) UpdateWeather(city string, weather *weatherv1.Weather) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.weathers[city] = weather
}

// WeatherHandler реализует интерфейс weatherv1.Handler для обработки запросов к API погоды
type WeatherHandler struct {
	storage *WeatherStorage
}

// NewWeatherHandler создает новый обработчик запросов к API погоды
func NewWeatherHandler(storage *WeatherStorage) *WeatherHandler {
	return &WeatherHandler{
		storage: storage,
	}
}

// GetWeatherByCity обрабатывает запрос на получение данных о погоде по названию города
func (h *WeatherHandler) GetWeatherByCity(_ context.Context, params weatherv1.GetWeatherByCityParams) (weatherv1.GetWeatherByCityRes, error) {
	weather := h.storage.GetWeather(params.City)
	if weather == nil {
		return &weatherv1.GetWeatherByCityNotFound{
			Code:    http.StatusNotFound,
			Message: "данные о погоде для города '" + params.City + "' не найдены",
		}, nil
	}

	return weather, nil
}

// UpdateWeatherByCity обрабатывает запрос на обновление данных о погоде по названию города
func (h *WeatherHandler) UpdateWeatherByCity(_ context.Context, req *weatherv1.UpdateWeatherRequest, params weatherv1.UpdateWeatherByCityParams) (weatherv1.UpdateWeatherByCityRes, error) {
	// Создаем объект погоды с полученными данными
	weather := &weatherv1.Weather{
		City:        params.City,
		Temperature: req.Temperature,
		UpdatedAt:   time.Now(),
	}

	// Обновляем данные в хранилище
	h.storage.UpdateWeather(params.City, weather)

	return weather, nil
}

// SetupRouter создает и настраивает HTTP роутер с middleware
func SetupRouter(weatherHandler *WeatherHandler) (chi.Router, error) {
	// Создаем OpenAPI сервер
	weatherServer, err := weatherv1.NewServer(weatherHandler)
	if err != nil {
		return nil, err
	}

	// Инициализируем роутер Chi
	r := chi.NewRouter()

	// Добавляем middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(middlewareTimeout))
	r.Use(customMiddleware.RequestLogger)

	// Монтируем обработчики OpenAPI на путь /api/*
	r.Handle("/api/*", weatherServer)

	return r, nil
}
