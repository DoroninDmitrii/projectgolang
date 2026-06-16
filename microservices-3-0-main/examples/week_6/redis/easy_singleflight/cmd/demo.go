package main

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_singleflight/internal/cache"
)

// demoStampede показывает проблему cache stampede
//
// Сценарий: «популярный товар» в интернет-магазине. Кеш пуст (TTL истёк
// или первый запуск). 50 горутин одновременно запрашивают один и тот же товар
//
// Что происходит:
//  1. Все 50 горутин проверяют кеш → cache miss
//  2. Все 50 идут в «медленную БД» (200ms на запрос)
//  3. Все 50 пишут одно и то же значение в кеш
//
// В продакшене: 50 одинаковых запросов к БД за 200ms — это нагрузка,
// которая может уронить БД при большом трафике (черная пятница, вирусный пост)
func demoStampede(
	ctx context.Context,
	productCache *cache.Cache,
	fetchFn cache.FetchFunc,
	dbCallCount *atomic.Int64,
	productID string,
	numGoroutines int,
) {
	slog.Info("============================================")
	slog.Info("=== Демо 1: ПРОБЛЕМА — cache stampede ===")
	slog.Info("============================================")
	slog.Info("кеш пуст, 50 горутин запрашивают один товар одновременно")

	// Очищаем кеш и счётчик
	if err := productCache.Del(ctx, productID); err != nil {
		slog.Error("ошибка очистки кеша", "error", err)
	}
	dbCallCount.Store(0)

	runConcurrent(numGoroutines, func() {
		_, err := productCache.GetProduct(ctx, productID, fetchFn)
		if err != nil {
			slog.Error("ошибка получения товара", "error", err)
		}
	})

	slog.Error(
		"ИТОГ: все горутины полезли в БД!",
		"goroutines", numGoroutines,
		"db_calls", dbCallCount.Load(),
	)
}

// demoSingleflight показывает решение через singleflight
//
// Тот же сценарий, но GetProductSF оборачивает вызов БД в singleflight.Group.Do()
//
// Что происходит:
//  1. Все 50 горутин проверяют кеш → cache miss
//  2. Первая горутина начинает запрос к БД через sf.Do("product:42", fn)
//  3. Остальные 49 вызывают sf.Do с тем же ключом — singleflight НЕ запускает
//     fn повторно, а блокирует их до завершения первого вызова
//  4. Первая горутина получает результат из БД, записывает в кеш
//  5. Все 50 горутин получают один и тот же результат
//
// Итого: 1 запрос к БД вместо 50.
func demoSingleflight(
	ctx context.Context,
	productCache *cache.Cache,
	fetchFn cache.FetchFunc,
	dbCallCount *atomic.Int64,
	productID string,
	numGoroutines int,
) {
	slog.Info("==============================================")
	slog.Info("=== Демо 2: РЕШЕНИЕ — singleflight ===")
	slog.Info("==============================================")
	slog.Info("тот же сценарий, но с singleflight.Group")

	// Очищаем кеш и счётчик
	if err := productCache.Del(ctx, productID); err != nil {
		slog.Error("ошибка очистки кеша", "error", err)
	}
	dbCallCount.Store(0)

	runConcurrent(numGoroutines, func() {
		_, err := productCache.GetProductSF(ctx, productID, fetchFn)
		if err != nil {
			slog.Error("ошибка получения товара", "error", err)
		}
	})

	slog.Info(
		"ИТОГ: singleflight защитил БД",
		"goroutines", numGoroutines,
		"db_calls", dbCallCount.Load(),
	)
}

// runConcurrent запускает N горутин, каждая выполняет fn
func runConcurrent(n int, fn func()) {
	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	wg.Wait()
}
