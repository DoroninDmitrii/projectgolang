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

	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/api/ufo/v1"
	weatherStub "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/client/stub/weather"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/service/ufo"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/pkg/proto/ufo/v1"
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

	// Stub-клиент погоды — возвращает захардкоженные данные для автономной работы
	// Для подключения к реальному сервису погоды см. internal/client/grpc/weather/v1/client.go:
	//
	//   weatherConn, err := grpc.NewClient("localhost:50052",
	//       grpc.WithTransportCredentials(insecure.NewCredentials()),
	//   )
	//   weatherGRPCClient := weatherv1.NewWeatherServiceClient(weatherConn)
	//   weatherClient := weatherClientv1.New(weatherGRPCClient)
	//
	weatherClient := weatherStub.New()

	// Регистрируем наш сервис
	repo := ufoRepository.NewRepository()
	service := ufoService.NewService(repo, weatherClient)
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
