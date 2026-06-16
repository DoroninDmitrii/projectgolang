package main

import (
	"cmp"
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-redis/redis_rate/v10"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/internal/api"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/internal/metrics"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/internal/ratelimit"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/pkg/proto/ufo/v1"
)

func main() {
	_ = godotenv.Load() //nolint:gosec // .env файл опционален

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	grpcHost := cmp.Or(os.Getenv("GRPC_HOST"), "localhost")
	grpcPort := cmp.Or(os.Getenv("GRPC_PORT"), "50051")
	grpcAddress := net.JoinHostPort(grpcHost, grpcPort)
	redisHost := cmp.Or(os.Getenv("REDIS_HOST"), "localhost")
	redisPort := cmp.Or(os.Getenv("REDIS_PORT"), "6379")
	redisAddr := net.JoinHostPort(redisHost, redisPort)

	// Инициализируем OTel Meter Provider (push метрик в OTel Collector через OTLP gRPC)
	// View переопределяет дефолтные бакеты для автоматической гистограммы otelgrpc
	// (rpc.server.call.duration) — дефолтные бакеты OTel SDK слишком крупные для in-memory сервиса
	histogramBuckets := []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5}
	metrics.Init(
		"distributed-rate-limiter-service",
		metrics.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: "rpc.server.call.duration"},
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

	// Подключение к Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer rdb.Close()

	limiter := redis_rate.NewLimiter(rdb)

	// Per-method лимиты RPS: запись — строже, чтение — мягче
	// PerSecond(n) — максимум n запросов в секунду (глобально на все инстансы)
	// ListUfo не указан явно — для него применится defaultLimit
	methodLimits := map[string]redis_rate.Limit{
		"/ufo.v1.UFOService/CreateUfo": redis_rate.PerSecond(5),  // 5 RPS
		"/ufo.v1.UFOService/GetUfo":    redis_rate.PerSecond(50), // 50 RPS
	}
	defaultLimit := redis_rate.PerSecond(10) // 10 RPS для остальных методов (ListUfo и т.д.)

	// otelgrpc.NewServerHandler автоматически записывает каждый gRPC вызов в гистограмму
	// rpc.server.call.duration (с лейблами: метод, статус-код и т.д.)
	// RPS считается на стороне Prometheus: rate(rpc_server_call_duration_seconds_count[1m])
	// Отклонённые rate limiter'ом запросы попадают со статус-кодом RESOURCE_EXHAUSTED
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(ratelimit.NewGRPCInterceptor(limiter, defaultLimit, methodLimits)),
	)
	ufov1.RegisterUFOServiceServer(grpcServer, api.NewAPI())
	reflection.Register(grpcServer)

	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("не удалось начать прослушивание", "address", grpcAddress, "error", err)
		return
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем gRPC-сервер в отдельной горутине
	go func() {
		slog.Info(
			"🚀 gRPC сервер запущен",
			"address", grpcAddress,
			"redis", redisAddr,
			"default_rps", defaultLimit.Rate,
		)
		if serveErr := grpcServer.Serve(listener); serveErr != nil {
			slog.Error("ошибка gRPC сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-ctx.Done()

	slog.Info("🛑 остановка gRPC сервера")
	grpcServer.GracefulStop()
	slog.Info("✅ gRPC сервер остановлен")
}
