package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	v1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/api/weather/v1"
	customMiddleware "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/middleware"
	weatherRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/repository/weather"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/openapi/weather/v1"
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

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Загружаем переменные окружения из .env
	err := godotenv.Load("http.env")
	if err != nil {
		slog.Error("ошибка загрузки переменных окружения из http.env", "error", err)
		return
	}

	// Подключаемся к PostgreSQL
	dbURI := os.Getenv("DB_URI")
	if dbURI == "" {
		slog.Error("переменная окружения DB_URI не установлена")
		return
	}

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("ошибка подключения к БД", "error", err)
		return
	}
	defer pool.Close()

	// Собираем зависимости
	repository := weatherRepo.NewRepository(pool)
	weatherAPI := v1.NewAPI(repository)

	// Создаем OpenAPI сервер
	weatherServer, err := weatherv1.NewServer(weatherAPI)
	if err != nil {
		slog.Error("ошибка создания сервера OpenAPI", "error", err)
		return
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

	// Запускаем HTTP-сервер с таймаутами для защиты от атак
	server := &http.Server{
		Addr:              net.JoinHostPort("localhost", httpPort),
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout, // Защита от Slowloris атаки
		ReadTimeout:       readTimeout,       // Лимит на чтение всего запроса
		WriteTimeout:      writeTimeout,      // Лимит на запись ответа
		IdleTimeout:       idleTimeout,       // Таймаут keep-alive соединений
	}

	// Запускаем сервер в отдельной горутине
	go func() {
		slog.Info("🌐 HTTP-сервер запущен на порту", "port", httpPort)
		if listenErr := server.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("ошибка запуска сервера", "error", listenErr)
			cancel()
		}
	}()

	// Ждём сигнал ОС или падение сервера
	<-ctx.Done()

	slog.Info("🛑 завершение работы сервера...")

	// Создаем контекст с таймаутом для остановки сервера
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	err = server.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("ошибка при остановке сервера", "error", err)
	}

	slog.Info("✅ сервер остановлен")
}
