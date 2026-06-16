// Пакет main — точка входа gRPC-сервера управления сессиями
package main

import (
	"context"
	"log/slog"
	"net"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/internal/interceptor"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/internal/server"
	sessionv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/pkg/proto/session/v1"
)

const grpcAddr = "localhost:50051"

func main() {
	// Создаём gRPC-сервер
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.Auth),
	)
	sessionv1.RegisterSessionServiceServer(grpcServer, server.NewServer())
	reflection.Register(grpcServer)

	// Создаём listener
	listener, err := net.Listen("tcp", grpcAddr) //nolint:noctx // easy-пример, net.Listen без контекста
	if err != nil {
		slog.Error("не удалось начать прослушивание", "addr", grpcAddr, "error", err)
		return
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем сервер в горутине
	go func() {
		slog.Info("🚀 gRPC сервер запущен", "addr", grpcAddr)
		if serveErr := grpcServer.Serve(listener); serveErr != nil {
			slog.Error("ошибка gRPC сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-ctx.Done()

	slog.Info("🛑 остановка gRPC сервера")
	grpcServer.GracefulStop()
	slog.Info("✅ сервер остановлен")
}
