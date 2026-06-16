package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	customMiddleware "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_chi_ogen/internal/middleware"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_chi_ogen/pkg/openapi/weather/v1"
)

const (
	httpPort = "8080"

	// Таймауты для HTTP-сервера
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
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

func setupRouter(weatherHandler *WeatherHandler) (chi.Router, error) {
	weatherServer, err := weatherv1.NewServer(weatherHandler)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания сервера OpenAPI: %w", err)
	}

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(middlewareTimeout))
	r.Use(customMiddleware.RequestLogger)

	r.Handle("/api/*", weatherServer)

	return r, nil
}

func main() {
	// Создаем хранилище для данных о погоде
	storage := NewWeatherStorage()

	// Создаем обработчик API погоды
	weatherHandler := NewWeatherHandler(storage)

	r, err := setupRouter(weatherHandler)
	if err != nil {
		slog.Error("ошибка создания роутера", "error", err)
		return
	}

	// Запускаем HTTP-сервер с таймаутами для защиты от атак
	// Подробное описание всех параметров: см. week_1/HTTP_SERVER.md
	server := &http.Server{
		Addr:              net.JoinHostPort("localhost", httpPort),
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout, // Защита от Slowloris атаки
		ReadTimeout:       readTimeout,       // Лимит на чтение всего запроса
		WriteTimeout:      writeTimeout,      // Лимит на запись ответа
		IdleTimeout:       idleTimeout,       // Таймаут keep-alive соединений
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем сервер в отдельной горутине
	go func() {
		slog.Info("🚀 HTTP-сервер запущен на порту", "port", httpPort)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("❌ ошибка запуска сервера", "error", serveErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Ждём сигнал от ОС или падение сервера
	<-ctx.Done()
	slog.Info("🛑 завершение работы сервера...")

	// Создаем контекст с таймаутом для остановки сервера
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("❌ ошибка при остановке сервера", "error", shutdownErr)
	}

	slog.Info("✅ сервер остановлен")
}
