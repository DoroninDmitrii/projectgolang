package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/platform/pkg/closer"
	platformRedis "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/platform/pkg/redis"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/api/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/config"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/ufo"
	ufoCacheRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/ufo_cache"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/service/ufo"
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
// Каждый геттер (PGPool, RedisClient, UFORepository, UFOCacheRepository, UFOService, UfoV1API)
// следует паттерну «ленивая инициализация» (lazy initialization):
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
	pgPool      *pgxpool.Pool
	redisClient *redis.Client

	// Репозитории
	ufoRepo      ufoService.UFORepository
	ufoCacheRepo ufoService.UFOCacheRepository

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

// RedisClient возвращает клиент Redis
// При первом вызове создаёт клиент и регистрирует closer
func (d *diContainer) RedisClient(_ context.Context) *redis.Client {
	if d.redisClient == nil {
		rdb, err := platformRedis.NewClient(&redis.Options{
			Addr:            config.AppConfig().Redis.Address(),
			DialTimeout:     config.AppConfig().Redis.ConnectionTimeout,
			ReadTimeout:     config.AppConfig().Redis.ConnectionTimeout,
			WriteTimeout:    config.AppConfig().Redis.ConnectionTimeout,
			MaxIdleConns:    config.AppConfig().Redis.MaxIdle,
			ConnMaxIdleTime: config.AppConfig().Redis.IdleTimeout,
		}, slog.Default())
		if err != nil {
			slog.Error("не удалось создать Redis клиент", "error", err)
			os.Exit(1)
		}

		closer.Add("Redis", func(_ context.Context) error {
			return rdb.Close()
		})

		d.redisClient = rdb
	}

	return d.redisClient
}

// UFORepository возвращает репозиторий наблюдений НЛО
func (d *diContainer) UFORepository(ctx context.Context) ufoService.UFORepository {
	if d.ufoRepo == nil {
		d.ufoRepo = ufoRepository.NewRepository(d.PGPool(ctx))
	}

	return d.ufoRepo
}

// UFOCacheRepository возвращает кеш-репозиторий наблюдений НЛО
func (d *diContainer) UFOCacheRepository(ctx context.Context) ufoService.UFOCacheRepository {
	if d.ufoCacheRepo == nil {
		d.ufoCacheRepo = ufoCacheRepository.NewRepository(d.RedisClient(ctx))
	}

	return d.ufoCacheRepo
}

// UFOService возвращает сервис бизнес-логики наблюдений НЛО
func (d *diContainer) UFOService(ctx context.Context) ufov1API.UFOService {
	if d.ufoSvc == nil {
		d.ufoSvc = ufoService.New(
			d.UFORepository(ctx),
			d.UFOCacheRepository(ctx),
			config.AppConfig().Redis.CacheTTL,
		)
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
