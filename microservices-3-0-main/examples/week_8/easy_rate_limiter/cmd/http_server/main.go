package main

import (
	"cmp"
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
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"golang.org/x/time/rate"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/internal/api"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/internal/metrics"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/internal/ratelimit"
)

const (
	rps   = 10.0
	burst = 20

	shutdownTimeout   = 5 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	_ = godotenv.Load() //nolint:gosec // .env файл опционален

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	httpHost := cmp.Or(os.Getenv("HTTP_HOST"), "localhost")
	httpPort := cmp.Or(os.Getenv("HTTP_PORT"), "8080")
	httpAddress := net.JoinHostPort(httpHost, httpPort)

	// Endpoint OTel Collector — единая правда для всех сигналов (метрик, трейсов, логов).
	// Берём из env, чтобы main был источником истины, а не SDK-дефолты внутри библиотеки
	collectorEndpoint := cmp.Or(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "localhost:4317")

	// Инициализируем OTel Meter Provider (push метрик в OTel Collector через OTLP gRPC)
	// View переопределяет дефолтные бакеты для автоматической гистограммы otelhttp
	// (http.server.request.duration) — дефолтные бакеты OTel SDK слишком крупные для in-memory сервиса
	histogramBuckets := []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5}
	metrics.Init(
		"rate-limiter-service",
		metrics.WithCollectorEndpoint(collectorEndpoint),
		metrics.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: "http.server.request.duration"},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: histogramBuckets,
				},
			},
		)),
	)
	defer func() {
		if err := metrics.Close(); err != nil {
			slog.Error("ошибка остановки metrics provider", "error", err)
		}
	}()

	handler := api.NewAPI()

	r := chi.NewRouter()
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Logger)
	// rate.NewLimiter создаёт token bucket rate limiter:
	//   - rps (rate.Limit) — максимальная средняя скорость: сколько токенов добавляется в секунду
	//     При rps=10 разрешается в среднем 10 запросов/сек
	//   - burst — размер корзины (максимум токенов). Разрешает кратковременные всплески:
	//     если корзина полная, можно пропустить burst запросов подряд без ожидания
	//     После всплеска скорость снова ограничивается значением rps
	r.Use(ratelimit.HTTPMiddleware(rate.NewLimiter(rate.Limit(rps), burst)))

	r.Get("/ping", handler.HandlePing)
	r.Post("/ufo", handler.HandleCreateUfo)
	r.Get("/ufo/{id}", handler.HandleGetUfo)

	// otelhttp.NewHandler оборачивает весь роутер и автоматически записывает каждый HTTP запрос
	// в гистограмму http.server.request.duration (с лейблами: маршрут, статус-код и т.д.)
	// RPS считается на стороне Prometheus: rate(http_server_request_duration_seconds_count[1m])
	// Отклонённые rate limiter'ом запросы попадают со статус-кодом 429 (Too Many Requests)
	srv := &http.Server{
		Addr:              httpAddress,
		Handler:           otelhttp.NewHandler(r, "rate-limiter-http"),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем HTTP-сервер в отдельной горутине
	go func() {
		slog.Info("🌐 HTTP сервер запущен", "address", httpAddress, "rps", rps, "burst", burst)
		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("ошибка HTTP сервера", "error", listenErr)
			cancel()
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-ctx.Done()

	slog.Info("🛑 получен сигнал остановки...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("ошибка остановки HTTP сервера", "error", shutdownErr)
	}

	slog.Info("✅ HTTP сервер остановлен")
}
