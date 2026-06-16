package main

import (
	"context"
	"log/slog"
	"net"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	classificationv1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/api/classification/v1"
	classificationService "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/service/classification"
	classificationServiceTracing "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/service/classification/tracing"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/platform/pkg/tracing"
	classificationv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/classification/v1"
)

const (
	grpcAddress               = ":50053"
	grpcMaxConnectionIdle     = 15 * time.Minute
	grpcMaxConnectionAge      = 30 * time.Minute
	grpcMaxConnectionAgeGrace = 5 * time.Second
	grpcKeepaliveTime         = 5 * time.Minute
	grpcKeepaliveTimeout      = 1 * time.Second
	grpcMinPingInterval       = 5 * time.Minute
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Инициализация трейсинга
	shutdownTracer, err := initTracing(ctx)
	if err != nil {
		slog.Error("не удалось инициализировать трейсинг", "error", err)
		return
	}
	defer func() {
		if cerr := shutdownTracer(ctx); cerr != nil {
			slog.Error("не удалось завершить трейсер", "error", cerr)
		}
	}()

	// Сборка зависимостей
	svc := classificationService.NewService()
	tracedSvc := classificationServiceTracing.NewTracedService(svc)
	api := classificationv1API.NewAPI(tracedSvc)

	// Создание gRPC сервера с трейсинг интерцептором
	server := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(tracing.TraceIDUnaryServerInterceptor()),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     grpcMaxConnectionIdle,
			MaxConnectionAge:      grpcMaxConnectionAge,
			MaxConnectionAgeGrace: grpcMaxConnectionAgeGrace,
			Time:                  grpcKeepaliveTime,
			Timeout:               grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcMinPingInterval,
			PermitWithoutStream: true,
		}),
	)

	// Регистрация сервиса
	classificationv1.RegisterClassificationServiceServer(server, api)
	reflection.Register(server)

	// Запуск слушателя
	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("не удалось начать прослушивание", "error", err)
		return
	}
	// defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	slog.Info("🚀 classification сервис запущен", "address", grpcAddress)

	// Запуск gRPC сервера в горутине
	go func() {
		if serveErr := server.Serve(lis); serveErr != nil {
			slog.Error("ошибка работы gRPC сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ожидание сигнала завершения или падения сервера
	<-ctx.Done()

	slog.Info("🛑 остановка classification сервера")
	server.GracefulStop()
	slog.Info("✅ classification сервер остановлен")
}
