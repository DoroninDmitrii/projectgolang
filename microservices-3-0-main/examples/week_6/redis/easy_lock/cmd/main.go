package main

import (
	"cmp"
	"context"
	"log/slog"
	"net"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
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

	// Четыре демо-сценария: сначала проблема, потом решения
	demoProblem(ctx, rdb)
	demoSolution(ctx, rdb)
	demoCompetition(ctx, rdb)
	demoTTLProtection(ctx, rdb)

	return nil
}
