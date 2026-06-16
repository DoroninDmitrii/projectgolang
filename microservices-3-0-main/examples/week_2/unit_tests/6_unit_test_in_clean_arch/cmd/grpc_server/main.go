package main

import (
	"context"
	"log/slog"
	"net"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/api/ufo/v1"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/service/ufo"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/pkg/proto/ufo/v1"
)

const grpcAddress = "localhost:50051"

func main() {
	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("не удалось создать слушатель", "error", err)
		return
	}
	// Примечание: defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	// Создаем gRPC сервер
	s := grpc.NewServer() // keepalive опущен для краткости — фокус на тестировании

	// Регистрируем наш сервис
	repo := ufoRepository.NewRepository()
	service := ufoService.NewService(repo)
	api := ufov1API.NewAPI(service)

	ufov1.RegisterUFOServiceServer(s, api)

	// Включаем рефлексию для отладки
	reflection.Register(s)

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		slog.Info("🚀 gRPC сервер запущен", "address", grpcAddress)
		if serveErr := s.Serve(lis); serveErr != nil {
			slog.Error("ошибка запуска сервера", "error", serveErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-ctx.Done()
	slog.Info("🛑 остановка gRPC сервера")
	s.GracefulStop()
	slog.Info("✅ сервер остановлен")
}
