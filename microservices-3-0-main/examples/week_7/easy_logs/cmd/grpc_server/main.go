package main

import (
	"context"
	"log/slog"
	"net"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/internal/handler"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/internal/interceptor"
	applogger "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/internal/logger"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/pkg/proto/ufo/v1"
)

const grpcAddress = "localhost:50051"

func main() {
	applogger.Init("info", true)
	defer func() {
		_ = applogger.Close() //nolint:gosec // ошибка закрытия логгера не критична при завершении
	}()

	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("ошибка запуска слушателя", "error", err)
		return
	}
	// Примечание: defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.LoggingInterceptor(),
		),
	)
	reflection.Register(s)
	ufov1.RegisterUFOServiceServer(s, handler.NewUFOServer())

	slog.Info("🚀 gRPC сервер запущен", "address", grpcAddress)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		if serveErr := s.Serve(lis); serveErr != nil {
			slog.Error("ошибка запуска сервера", "error", serveErr)
			cancel()
		}
	}()

	<-ctx.Done()

	slog.Info("🛑 остановка gRPC сервера")
	s.GracefulStop()
	slog.Info("✅ gRPC сервер остановлен")
}
