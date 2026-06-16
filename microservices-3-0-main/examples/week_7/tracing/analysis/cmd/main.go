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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	analysisv1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/api/analysis/v1"
	classificationClient "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/client/grpc/classification"
	ufoClient "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/client/grpc/ufo"
	analysisService "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/service/analysis"
	analysisServiceTracing "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/analysis/internal/service/analysis/tracing"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/platform/pkg/tracing"
	analysisv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/analysis/v1"
	classificationv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/classification/v1"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/ufo/v1"
)

const (
	grpcAddress               = ":50052"
	ufoServiceAddress         = "localhost:50051"
	classificationAddress     = "localhost:50053"
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

	// Подключение к UFO сервису
	// otelgrpc.NewClientHandler() автоматически создаёт спаны для каждого исходящего gRPC вызова,
	// прокидывает trace context в метадату и записывает статус/ошибки
	ufoConn, err := grpc.NewClient(
		ufoServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		slog.Error("не удалось подключиться к UFO сервису", "error", err)
		return
	}

	// Подключение к Classification сервису (аналогично — автотрейсинг исходящих вызовов)
	classificationConn, err := grpc.NewClient(
		classificationAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		slog.Error("не удалось подключиться к Classification сервису", "error", err)
		return
	}

	// Сборка зависимостей
	ufoCl := ufoClient.New(ufov1.NewUFOServiceClient(ufoConn))
	classificationCl := classificationClient.New(classificationv1.NewClassificationServiceClient(classificationConn))

	svc := analysisService.NewService(ufoCl, classificationCl)
	tracedSvc := analysisServiceTracing.NewTracedService(svc)
	api := analysisv1API.NewAPI(tracedSvc)

	// Создание gRPC сервера с трейсинг интерцептором
	// otelgrpc.NewServerHandler() автоматически создаёт спаны для каждого входящего gRPC запроса,
	// извлекает trace context из метадаты и записывает статус/ошибки
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
	analysisv1.RegisterAnalysisServiceServer(server, api)
	reflection.Register(server)

	// Запуск слушателя
	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("не удалось начать прослушивание", "error", err)
		return
	}
	// defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	slog.Info("🚀 analysis сервис запущен", "address", grpcAddress)

	// Запуск gRPC сервера в горутине
	go func() {
		if serveErr := server.Serve(lis); serveErr != nil {
			slog.Error("ошибка работы gRPC сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ожидание сигнала завершения или падения сервера
	<-ctx.Done()

	slog.Info("🛑 остановка analysis сервера")
	server.GracefulStop()
	slog.Info("✅ analysis сервер остановлен")
}
