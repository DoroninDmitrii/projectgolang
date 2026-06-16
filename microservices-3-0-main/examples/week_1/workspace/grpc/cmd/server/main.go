package main

import (
	"context"
	"log/slog"
	"net"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/grpc/pkg/service"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/shared/pkg/proto/ufo/v1"
)

const (
	// Адрес сервера
	grpcAddress = "localhost:50051"

	// gRPC keepalive параметры
	grpcMaxConnectionIdle     = 15 * time.Minute // Закрыть idle-соединения (нет активных RPC)
	grpcMaxConnectionAge      = 30 * time.Minute // Принудительная ротация для балансировки
	grpcMaxConnectionAgeGrace = 5 * time.Second  // Время на завершение активных RPC
	grpcKeepaliveTime         = 5 * time.Minute  // Интервал ping'ов для обнаружения мёртвых соединений
	grpcKeepaliveTimeout      = 1 * time.Second  // Таймаут ожидания pong
	grpcMinPingInterval       = 5 * time.Minute  // Минимальный интервал ping'ов от клиента (защита от DoS)
)

func main() {
	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("ошибка запуска слушателя", "error", err)
		return
	}
	// Примечание: defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	// Создаем gRPC сервер с keepalive настройками
	// Подробное описание всех параметров: см. week_1/GRPC_CONNECTIONS.md
	s := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     grpcMaxConnectionIdle,
			MaxConnectionAge:      grpcMaxConnectionAge,
			MaxConnectionAgeGrace: grpcMaxConnectionAgeGrace,
			Time:                  grpcKeepaliveTime,
			Timeout:               grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcMinPingInterval,
			PermitWithoutStream: true, // Разрешить "тёплые" соединения без активных RPC
		}),
	)

	// Регистрируем наш сервис
	ufov1.RegisterUFOServiceServer(s, ufoService.New())

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

	// Ждём сигнал от ОС или падение сервера
	<-ctx.Done()
	slog.Info("🛑 остановка gRPC сервера")
	s.GracefulStop()
	slog.Info("✅ сервер остановлен")
}
