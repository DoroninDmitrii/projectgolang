package main

import (
	"cmp"
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/handler"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/interceptor"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/metrics"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/pkg/proto/ufo/v1"
)

func main() {
	// Загружаем переменные окружения из .env (если файл существует)
	_ = godotenv.Load() //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	grpcHost := cmp.Or(os.Getenv("GRPC_HOST"), "localhost")
	grpcPort := cmp.Or(os.Getenv("GRPC_PORT"), "50051")
	grpcAddress := net.JoinHostPort(grpcHost, grpcPort)

	// Инициализируем OTel Meter Provider (push метрик в OTel Collector через OTLP gRPC)
	metrics.Init()
	defer func() {
		if err := metrics.Close(); err != nil {
			slog.Error("ошибка остановки metrics provider", "error", err)
		}
	}()

	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("ошибка запуска слушателя", "error", err)
		return
	}
	// Примечание: defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	// gRPC сервер с otelgrpc StatsHandler — автоматический сбор gRPC метрик:
	//   - rpc.server.duration        — длительность запросов (histogram)
	//   - rpc.server.request.size    — размер запросов (histogram)
	//   - rpc.server.response.size   — размер ответов (histogram)
	//   - rpc.server.requests_per_rpc  — сообщений на запрос
	//   - rpc.server.responses_per_rpc — сообщений на ответ
	//
	// Все метрики экспортируются через тот же OTel Meter Provider → OTLP → Collector → Prometheus
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			interceptor.MetricsInterceptor(), // кастомная Histogram-метрика длительности запросов
		),
	)
	reflection.Register(grpcServer)
	ufov1.RegisterUFOServiceServer(grpcServer, handler.NewUFOServer())

	slog.Info("🚀 gRPC сервер запущен", "address", grpcAddress)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			slog.Error("ошибка запуска сервера", "error", serveErr)
			cancel()
		}
	}()

	<-ctx.Done()

	slog.Info("🛑 остановка gRPC сервера")
	grpcServer.GracefulStop()
	slog.Info("✅ gRPC сервер остановлен")
}
