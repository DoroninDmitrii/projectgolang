package app

import (
	"context"
	"log/slog"
	"os"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"

	pcBuilderAPI "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/api/pc_builder"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/config"
	buildRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/build"
	componentRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/component"
	pcBuilder "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/service/application/pc_builder"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/service/domain"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/platform/pkg/closer"
)

// diContainer — контейнер зависимостей с ленивой инициализацией
// Каждый геттер проверяет nil, создаёт объект при первом вызове и кэширует
type diContainer struct {
	// Инфраструктура
	pgPool    *pgxpool.Pool
	txManager pcBuilder.TxManager

	// Репозитории
	componentRepo pcBuilder.ComponentRepository
	buildRepo     pcBuilder.BuildRepository

	// Доменный сервис
	checker pcBuilder.CompatibilityChecker

	// Application-сервис
	pcBuilderSvc pcBuilderAPI.PCBuilderService

	// API-обработчик
	handler *pcBuilderAPI.Handler
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

		if err = pool.Ping(ctx); err != nil {
			slog.Error("не удалось выполнить ping PostgreSQL", "error", err)
			os.Exit(1)
		}

		closer.Add("PostgreSQL pool", func(_ context.Context) error {
			pool.Close()
			return nil
		})

		d.pgPool = pool
	}

	return d.pgPool
}

// TxManager возвращает менеджер транзакций
func (d *diContainer) TxManager(ctx context.Context) pcBuilder.TxManager {
	if d.txManager == nil {
		m, err := manager.New(trmpgx.NewDefaultFactory(d.PGPool(ctx)))
		if err != nil {
			slog.Error("не удалось создать transaction manager", "error", err)
			os.Exit(1)
		}

		d.txManager = m
	}

	return d.txManager
}

// ComponentRepository возвращает репозиторий комплектующих
func (d *diContainer) ComponentRepository(ctx context.Context) pcBuilder.ComponentRepository {
	if d.componentRepo == nil {
		d.componentRepo = componentRepo.NewRepository(d.PGPool(ctx))
	}

	return d.componentRepo
}

// BuildRepository возвращает репозиторий сборок
func (d *diContainer) BuildRepository(ctx context.Context) pcBuilder.BuildRepository {
	if d.buildRepo == nil {
		d.buildRepo = buildRepo.NewRepository(d.PGPool(ctx))
	}

	return d.buildRepo
}

// CompatibilityChecker возвращает доменный сервис проверки совместимости
func (d *diContainer) CompatibilityChecker() pcBuilder.CompatibilityChecker {
	if d.checker == nil {
		d.checker = domain.NewCompatibilityChecker()
	}

	return d.checker
}

// PCBuilderService возвращает application-сервис сборки ПК
func (d *diContainer) PCBuilderService(ctx context.Context) pcBuilderAPI.PCBuilderService {
	if d.pcBuilderSvc == nil {
		d.pcBuilderSvc = pcBuilder.NewService(
			d.TxManager(ctx),
			d.ComponentRepository(ctx),
			d.BuildRepository(ctx),
			d.CompatibilityChecker(),
		)
	}

	return d.pcBuilderSvc
}

// PCBuilderHandler возвращает API-обработчик сборки ПК
func (d *diContainer) PCBuilderHandler(ctx context.Context) *pcBuilderAPI.Handler {
	if d.handler == nil {
		d.handler = pcBuilderAPI.NewHandler(d.PCBuilderService(ctx))
	}

	return d.handler
}
