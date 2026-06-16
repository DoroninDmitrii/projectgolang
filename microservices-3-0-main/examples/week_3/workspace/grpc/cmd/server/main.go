package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	v1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/api/ufo/v1"
	ufoRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/repository/ufo"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/proto/ufo/v1"
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
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Загружаем переменные окружения из .env
	err := godotenv.Load("grpc.env")
	if err != nil {
		slog.Error("ошибка загрузки переменных окружения из grpc.env", "error", err)
		return
	}

	// Подключаемся к PostgreSQL
	dbURI := os.Getenv("DB_URI")
	if dbURI == "" {
		slog.Error("переменная окружения DB_URI не установлена")
		return
	}

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("ошибка подключения к БД", "error", err)
		return
	}
	defer pool.Close()

	// Собираем зависимости
	repository := ufoRepo.NewRepository(pool)
	ufoAPI := v1.NewAPI(repository)

	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("ошибка запуска слушателя", "error", err)
		return
	}
	// Примечание: defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	// Создаем gRPC сервер с keepalive настройками
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
	ufov1.RegisterUFOServiceServer(s, ufoAPI)

	// Включаем рефлексию для отладки
	reflection.Register(s)

	go func() {
		slog.Info("🚀 gRPC сервер запущен", "address", grpcAddress)
		if serveErr := s.Serve(lis); serveErr != nil {
			slog.Error("ошибка запуска сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ждём сигнал ОС или падение сервера
	<-ctx.Done()
	slog.Info("🛑 остановка gRPC сервера")
	s.GracefulStop()
	slog.Info("✅ сервер остановлен")
}
