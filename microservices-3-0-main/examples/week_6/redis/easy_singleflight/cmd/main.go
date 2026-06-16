package main

import (
	"cmp"
	"context"
	"log/slog"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_singleflight/internal/cache"
)

func main() {
	if err := run(); err != nil {
		slog.Error("критическая ошибка", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Загружаем переменные окружения из .env (если файл существует)
	_ = godotenv.Load() //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	// Подключаемся к Redis
	redisHost := cmp.Or(os.Getenv("REDIS_HOST"), "localhost")
	redisPort := cmp.Or(os.Getenv("REDIS_PORT"), "6379")

	addr := net.JoinHostPort(redisHost, redisPort)

	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			slog.Error("не удалось закрыть соединение с Redis", "error", err)
		}
	}()

	ctx := context.Background()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	slog.Info("подключились к Redis", "addr", addr)

	const (
		numGoroutines = 50
		productID     = "popular-item-42"
		cacheTTL      = 30 * time.Second
	)

	productCache := cache.New(rdb, cacheTTL)

	// fetchFromDB симулирует медленный запрос к базе данных
	var dbCallCount atomic.Int64

	fetchFromDB := func(ctx context.Context, productID string) (*cache.Product, error) {
		dbCallCount.Add(1)

		// Симуляция медленного запроса
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		return &cache.Product{
			ID:    productID,
			Name:  "Популярный товар",
			Price: 9990,
		}, nil
	}

	// Демо 1: проблема — cache stampede без singleflight
	demoStampede(ctx, productCache, fetchFromDB, &dbCallCount, productID, numGoroutines)

	// Демо 2: решение — singleflight группирует запросы
	demoSingleflight(ctx, productCache, fetchFromDB, &dbCallCount, productID, numGoroutines)

	return nil
}
