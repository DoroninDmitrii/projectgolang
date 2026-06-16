package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_retry/internal/api"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_retry/pkg/proto/ufo/v1"
)

const grpcPort = "localhost:50051"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	grpcServer := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(grpcServer, api.NewAPI())
	reflection.Register(grpcServer)

	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	listener, err := net.Listen("tcp", grpcPort)
	if err != nil {
		slog.Error("не удалось начать прослушивание", "port", grpcPort, "error", err)
		return
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		slog.Info("🚀 gRPC сервер запущен (первые запросы будут сбоить)", "port", grpcPort)
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
