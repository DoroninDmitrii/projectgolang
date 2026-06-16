package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/platform/pkg/closer"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/api/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/config"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/service/ufo"
)

// diContainer — контейнер зависимостей (Composition Root) приложения
//
// Зачем это нужно:
// В простых приложениях зависимости создаются прямо в main.go: pool := pgxpool.New(...),
// repo := NewRepo(pool), svc := NewService(repo) и т.д. Это работает, пока зависимостей мало
// Когда сервис обрастает десятками компонентов, main.go превращается в «простыню» инициализации,
// а порядок создания начинает зависеть от неочевидных связей
//
// DI-контейнер решает эту проблему: каждый компонент «знает», от чего зависит, и создаёт
// свои зависимости по цепочке автоматически при первом обращении
//
// Как это работает:
// Каждый геттер (PGPool, UFORepository, UFOService, UfoV1API) следует паттерну
// «ленивая инициализация» (lazy initialization):
//  1. Проверяет, создан ли уже объект (nil-check)
//  2. Если нет — создаёт, запоминает в поле и возвращает
//  3. Если да — сразу возвращает ранее созданный экземпляр
//
// Это гарантирует, что каждый компонент создаётся ровно один раз, независимо от того,
// сколько раз к нему обращаются, и в правильном порядке
//
// Как добавить новую зависимость:
//  1. Добавьте поле с типом интерфейса в структуру
//  2. Напишите геттер с nil-check, который вызывает геттеры зависимостей
//  3. Используйте геттер там, где нужен компонент
//
// Почему интерфейсы (а не конкретные типы):
// Структуры слоёв (repository, service, api) — unexported, чтобы их нельзя было создать
// в обход конструктора New(). Контейнер хранит интерфейсы, которые определены в потребителях
// (deps.go). Это также позволяет легко подменять реализации при необходимости
//
// Почему геттеры не возвращают ошибки:
// Если не удалось подключиться к базе — приложение не может работать. Вместо того,
// чтобы протаскивать ошибку через 5 уровней вызовов, мы логируем и завершаем процесс
// сразу в месте проблемы. Это упрощает API контейнера и код всех вызывающих
type diContainer struct {
	// Инфраструктура
	pgPool *pgxpool.Pool

	// Репозитории
	ufoRepo ufoService.UFORepository

	// Сервисы
	ufoSvc ufov1API.UFOService

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

		closer.Add("PostgreSQL pool", func(_ context.Context) error {
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
		d.ufoRepo = ufoRepository.New(d.PGPool(ctx))
	}

	return d.ufoRepo
}

// UFOService возвращает сервис бизнес-логики наблюдений НЛО
func (d *diContainer) UFOService(ctx context.Context) ufov1API.UFOService {
	if d.ufoSvc == nil {
		d.ufoSvc = ufoService.New(d.UFORepository(ctx))
	}

	return d.ufoSvc
}

// UfoV1API возвращает gRPC-обработчик сервиса наблюдений НЛО
func (d *diContainer) UfoV1API(ctx context.Context) ufov1.UFOServiceServer {
	if d.ufov1Handler == nil {
		d.ufov1Handler = ufov1API.New(d.UFOService(ctx))
	}

	return d.ufov1Handler
}
