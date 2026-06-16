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

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/api/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/config"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/ufo/internal/service/ufo"
)

const (
	shutdownTimeout = 5 * time.Second

	grpcMaxConnectionIdle     = 15 * time.Minute
	grpcMaxConnectionAge      = 30 * time.Minute
	grpcMaxConnectionAgeGrace = 5 * time.Second
	grpcKeepaliveTime         = 5 * time.Minute
	grpcKeepaliveTimeout      = 1 * time.Second
	grpcMinPingInterval       = 5 * time.Minute
)

func main() {
	if err := run(); err != nil {
		slog.Error("приложение завершилось с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// Загружаем переменные окружения из .env файла (если он существует)
	// Они попадают в os.Environ и далее перетирают значения из YAML-конфига
	// Приоритет: системные env > .env файл > yaml > env-default
	_ = godotenv.Load("../ufo.env") //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	// Определяем путь к конфиг-файлу: флаг -config > env CONFIG_PATH > config.local.yaml
	configPath := config.ResolveConfigPath()

	// Загружаем конфигурацию: сначала YAML, затем env-переменные поверх
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	slog.Info(
		"конфигурация загружена",
		"config_path", configPath,
		"grpc_address", cfg.GRPC.Address(),
		"pg_host", cfg.PG.Host,
	)

	// Подключаемся к PostgreSQL
	pool, err := pgxpool.New(ctx, cfg.PG.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()

	// Проверяем подключение
	if err = pool.Ping(ctx); err != nil {
		return err
	}
	slog.Info("подключение к PostgreSQL установлено")

	// Собираем слои приложения (repository → service → API)
	repo := ufoRepository.NewRepository(pool)
	svc := ufoService.NewService(repo)
	api := ufov1API.NewAPI(svc)

	// Создаём gRPC-сервер
	grpcServer := grpc.NewServer(
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
	reflection.Register(grpcServer)
	ufov1.RegisterUFOServiceServer(grpcServer, api)

	// Запускаем gRPC-сервер в отдельной горутине
	addr := cfg.GRPC.Address()

	lis, err := net.Listen("tcp", addr) //nolint:noctx // net.Listen не требует контекст, адрес из конфига
	if err != nil {
		return err
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	shutdownCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		slog.Info("🚀 gRPC-сервер запущен", "address", addr)
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			slog.Error("ошибка gRPC-сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-shutdownCtx.Done()

	slog.Info("получен сигнал завершения, останавливаем сервер...")

	// GracefulStop у gRPC не принимает контекст, поэтому используем select с таймаутом
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		slog.Info("✅ gRPC-сервер остановлен")
	case <-time.After(shutdownTimeout):
		slog.Info("превышен таймаут graceful shutdown, принудительная остановка")
		grpcServer.Stop()
	}

	return nil
}
