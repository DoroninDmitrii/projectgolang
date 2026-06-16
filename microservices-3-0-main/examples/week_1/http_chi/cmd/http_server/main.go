package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_chi/pkg/models"
)

const (
	httpPort     = "8080"
	urlParamCity = "city"

	// Таймауты для HTTP-сервера
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
	middlewareTimeout = 10 * time.Second
)

func setupRouter(storage *models.WeatherStorage) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(middlewareTimeout))
	r.Use(render.SetContentType(render.ContentTypeJSON))

	r.Route("/api/v1/weather", func(r chi.Router) {
		r.Get("/{city}", getWeatherHandler(storage))
		r.Put("/{city}", updateWeatherHandler(storage))
	})

	return r
}

func main() {
	// Создаем хранилище для данных о погоде
	storage := models.NewWeatherStorage()

	r := setupRouter(storage)

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
		listenErr := server.ListenAndServe()
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("❌ ошибка запуска сервера", "error", listenErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-ctx.Done()
	slog.Info("🛑 завершение работы сервера...")

	// Создаем контекст с таймаутом для остановки сервера
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	err := server.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("❌ ошибка при остановке сервера", "error", err)
	}

	slog.Info("✅ сервер остановлен")
}

// getWeatherHandler обрабатывает запросы на получение информации о погоде для города
func getWeatherHandler(storage *models.WeatherStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		city := chi.URLParam(r, urlParamCity)
		if city == "" {
			http.Error(w, "параметр city обязателен", http.StatusBadRequest)
			return
		}

		weather := storage.GetWeather(city)
		if weather == nil {
			http.Error(w, fmt.Sprintf("данные о погоде для города '%s' не найдены", city), http.StatusNotFound)
			return
		}

		render.JSON(w, r, weather)
	}
}

// updateWeatherHandler обрабатывает запросы на обновление информации о погоде для города
func updateWeatherHandler(storage *models.WeatherStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		city := chi.URLParam(r, urlParamCity)
		if city == "" {
			http.Error(w, "параметр city обязателен", http.StatusBadRequest)
			return
		}

		// Декодируем данные из тела запроса
		var weatherUpdate models.Weather
		if err := json.NewDecoder(r.Body).Decode(&weatherUpdate); err != nil {
			http.Error(w, "некорректное тело запроса", http.StatusBadRequest)
			return
		}

		// Устанавливаем имя города из URL-параметра
		weatherUpdate.City = city

		// Устанавливаем время обновления
		weatherUpdate.UpdatedAt = time.Now()

		// Обновляем информацию о погоде
		storage.UpdateWeather(&weatherUpdate)

		// Возвращаем обновленные данные
		render.JSON(w, r, weatherUpdate)
	}
}
