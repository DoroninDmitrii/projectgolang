package main

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// demoProblem показывает проблему: без распределённой блокировки несколько
// экземпляров сервиса дублируют обработку одного и того же заказа
//
// Сценарий: интернет-магазин, 5 экземпляров order-service слушают Kafka-топик
// Заказ #123 попадает ко всем (fan-out, retry, at-least-once delivery)
// Каждый экземпляр самостоятельно списывает деньги с баланса в Redis
// Итог: баланс уменьшается на 5000 вместо 1000.
func demoProblem(ctx context.Context, rdb *redis.Client) {
	slog.Info("========================================")
	slog.Info("=== Демо 1: ПРОБЛЕМА — без блокировки ===")
	slog.Info("========================================")

	const (
		numWorkers   = 5
		userKey      = "user:42"
		fieldBalance = "balance"
		orderCost    = 1000
		initBalance  = 5000
	)

	// Создаём пользователя с балансом 5000 рублей (хеш с несколькими полями)
	rdb.HSet(ctx, userKey, map[string]any{
		"name":       "Иван",
		"email":      "ivan@example.com",
		fieldBalance: initBalance,
	})
	defer rdb.Del(ctx, userKey)

	slog.Info("пользователь создан", "key", userKey, "balance", initBalance)
	slog.Info("стоимость заказа", "cost", orderCost)
	slog.Info("5 экземпляров сервиса получили заказ #123 из Kafka")

	var (
		wg             sync.WaitGroup
		processedCount atomic.Int64
	)

	for i := range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Каждый экземпляр сервиса независимо списывает деньги из поля balance
			// HIncrBy(key, field, increment) — выполняет HINCRBY: атомарно увеличивает
			// числовое поле хеша на increment (отрицательное значение = списание)
			// Сама операция атомарна, но проблема не в ней — проблема в том,
			// что заказ обрабатывается несколько раз целиком
			newBalance, err := rdb.HIncrBy(ctx, userKey, fieldBalance, -orderCost).Result()
			if err != nil {
				slog.Error("ошибка списания", "worker", i, "error", err)
				return
			}

			processedCount.Add(1)
			slog.Warn("работник списал деньги (дубль!)",
				"worker", i, "order", 123, "new_balance", newBalance)
		}()
	}

	wg.Wait()

	finalBalance, err := rdb.HGet(ctx, userKey, fieldBalance).Int64()
	if err != nil {
		slog.Error("ошибка чтения баланса", "error", err)
		return
	}

	slog.Error(
		"ИТОГ: деньги списаны несколько раз!",
		"processed_count", processedCount.Load(),
		"final_balance", finalBalance,
		"expected_balance", initBalance-orderCost,
	)
}
