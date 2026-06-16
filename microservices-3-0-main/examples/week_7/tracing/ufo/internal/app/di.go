package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/platform/pkg/closer"
	analysisv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/analysis/v1"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/api/ufo/v1"
	analysisClient "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/client/grpc/analysis"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/config"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/ufo"
	ufoServiceTracing "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/ufo/tracing"
)

// diContainer — контейнер зависимостей (Composition Root) приложения
//
// Каждый геттер следует паттерну «ленивая инициализация»:
//  1. Проверяет, создан ли уже объект (nil-check)
//  2. Если нет — создаёт, запоминает в поле и возвращает
//  3. Если да — сразу возвращает ранее созданный экземпляр
type diContainer struct {
	// Инфраструктура
	pgPool *pgxpool.Pool

	// Репозитории
	ufoRepo ufoService.UFORepository

	// Сервисы
	ufoSvc ufov1API.UFOService

	// Клиенты
	analysisGRPCClient ufoService.AnalysisClient

	// API-обработчики
	ufov1Handler ufov1.UFOServiceServer
}

// PGPool возвращает пул подключений к PostgreSQL
// При первом вызове создаёт пул, проверяет соединение и регистрирует closer
func (d *diContainer) PGPool(ctx context.Context) *pgxpool.Pool {
	if d.pgPool == nil {
		pool, err := pgxpool.New(ctx, config.AppConfig().PG.DSN())
		if err != nil {
			slog.Error("не удалось подключиться к PostgreSQL", "error", err)
			os.Exit(1)
		}

		err = pool.Ping(ctx)
		if err != nil {
			slog.Error("не удалось выполнить ping PostgreSQL", "error", err)
			os.Exit(1)
		}

		closer.Add("пул соединений PostgreSQL", func(_ context.Context) error {
			pool.Close()
			return nil
		})

		d.pgPool = pool
	}

	return d.pgPool
}

// UFORepository возвращает репозиторий наблюдений НЛО
func (d *diContainer) UFORepository(ctx context.Context) ufoService.UFORepository {
	if d.ufoRepo == nil {
		d.ufoRepo = ufoRepository.NewRepository(d.PGPool(ctx))
	}

	return d.ufoRepo
}

// UFOService возвращает сервис бизнес-логики наблюдений НЛО
func (d *diContainer) UFOService(ctx context.Context) ufov1API.UFOService {
	if d.ufoSvc == nil {
		svc := ufoService.NewService(d.UFORepository(ctx), d.AnalysisClient(ctx))
		d.ufoSvc = ufoServiceTracing.NewTracedService(svc)
	}

	return d.ufoSvc
}

// AnalysisClient возвращает gRPC-клиент сервиса анализа
func (d *diContainer) AnalysisClient(_ context.Context) ufoService.AnalysisClient {
	if d.analysisGRPCClient == nil {
		conn, err := grpc.NewClient(
			config.AppConfig().GRPC.AnalysisAddress(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		)
		if err != nil {
			slog.Error("не удалось создать соединение с analysis-сервисом", "error", err)
			os.Exit(1)
		}

		closer.Add("gRPC соединение с analysis-сервисом", func(_ context.Context) error {
			return conn.Close()
		})

		protoClient := analysisv1.NewAnalysisServiceClient(conn)
		d.analysisGRPCClient = analysisClient.New(protoClient)
	}

	return d.analysisGRPCClient
}

// UfoV1API возвращает gRPC-обработчик сервиса наблюдений НЛО
func (d *diContainer) UfoV1API(ctx context.Context) ufov1.UFOServiceServer {
	if d.ufov1Handler == nil {
		d.ufov1Handler = ufov1API.NewAPI(d.UFOService(ctx))
	}

	return d.ufov1Handler
}
