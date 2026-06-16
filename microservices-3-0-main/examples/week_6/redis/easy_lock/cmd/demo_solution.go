package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_lock/internal/lock"
)

// onceExecutor — интерфейс для OnceExecutor (определяем на стороне потребителя, а не реализации)
// "Accept interfaces, return structs" — идиоматичный Go-подход
type onceExecutor interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// demoSolution показывает решение: distributed lock + idempotency key
//
// Два отдельных механизма работают вместе:
//
//  1. Distributed Lock (lockKey) — мьютекс. Защищает от одновременной обработки
//     Захватываем перед работой, освобождаем после — как и рекомендует
//     официальная документация Redis:
//     https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/
//
//  2. Idempotency Key (doneKey) — маркер «заказ уже обработан»
//     Ставится ПОСЛЕ успешной обработки. Не даёт обработать заказ повторно
//     TTL = 24 часа — достаточно, чтобы пережить все retry из Kafka,
//     и не захламляет Redis навечно
//
// Зачем два ключа, а не один?
//   - Если использовать только лок без Release — он не захламляет Redis (TTL удалит),
//     но при крэше ДО обработки ключ «застрянет» до истечения TTL,
//     блокируя повторные попытки
//   - Если использовать только лок с Release — другой работник может захватить
//     освободившуюся блокировку и обработать заказ повторно
//   - Lock + Idempotency Key: лок освобождается сразу (не захламляет Redis),
//     а doneKey хранит факт обработки и не даёт повторить
func demoSolution(ctx context.Context, rdb *redis.Client) {
	slog.Info("==========================================")
	slog.Info("=== Демо 2: РЕШЕНИЕ — Lock + Idempotency Key ===")
	slog.Info("==========================================")
	slog.Info("те же 5 работников, но теперь с блокировкой и ключом идемпотентности")

	const (
		numWorkers   = 5
		userKey      = "user:43"
		fieldBalance = "balance"
		orderCost    = 1000
		initBalance  = 5000
		lockKey      = "order:123:lock" // Мьютекс — защита от одновременной обработки
		doneKey      = "order:123:done" // Маркер — заказ уже обработан
		lockTTL      = 10 * time.Second // TTL лока — страховка от deadlock при крэше
		doneTTL      = 24 * time.Hour   // TTL маркера — сколько помним, что заказ обработан
	)

	// Создаём пользователя с балансом
	rdb.HSet(ctx, userKey, map[string]any{
		"name":       "Пётр",
		"email":      "petr@example.com",
		fieldBalance: initBalance,
	})
	defer rdb.Del(ctx, userKey)
	defer rdb.Del(ctx, lockKey)
	defer rdb.Del(ctx, doneKey)

	slog.Info("пользователь создан", "key", userKey, "balance", initBalance)

	// OnceExecutor — аналог sync.Once, но распределённый
	// Внутри: Lock (мьютекс) + Idempotency Key (маркер «уже сделано») + double-check
	var once onceExecutor = lock.NewOnceExecutor(rdb, lockKey, lockTTL, doneKey, doneTTL)

	var (
		wg             sync.WaitGroup
		processedCount atomic.Int64
	)

	for i := range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// once.Do выполнит fn ровно один раз среди всех конкурирующих горутин
			// Остальные получат ErrAlreadyDone или ErrLockBusy
			err := once.Do(ctx, func(ctx context.Context) error {
				_, hErr := rdb.HIncrBy(ctx, userKey, fieldBalance, -orderCost).Result()
				return hErr
			})

			switch {
			case nil == err:
				processedCount.Add(1)
				slog.Info("заказ обработан", "worker", i)
			case errors.Is(err, lock.ErrAlreadyDone):
				slog.Info("заказ уже обработан, пропускаю", "worker", i)
			case errors.Is(err, lock.ErrLockBusy):
				slog.Info("блокировка занята, пропускаю", "worker", i)
			default:
				slog.Error("ошибка обработки заказа", "worker", i, "error", err)
			}
		}()
	}

	wg.Wait()

	finalBalance, err := rdb.HGet(ctx, userKey, fieldBalance).Int64()
	if err != nil {
		slog.Error("ошибка чтения баланса", "error", err)
		return
	}

	slog.Info(
		"ИТОГ: заказ обработан ровно 1 раз",
		"processed_count", processedCount.Load(),
		"final_balance", finalBalance,
		"expected_balance", initBalance-orderCost,
	)
	slog.Info("")
}
