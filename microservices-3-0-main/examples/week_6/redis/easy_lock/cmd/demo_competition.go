package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_lock/internal/lock"
)

// demoCompetition показывает очередь работников: все ждут своей очереди
// через AcquireWithRetry, каждый обрабатывает заказ последовательно
//
// Отличие от demoSolution: там работники сразу уходят, если блокировка занята
// Здесь — ждут освобождения и обрабатывают по очереди (полезно для фоновых задач)
func demoCompetition(ctx context.Context, rdb *redis.Client) {
	slog.Info("============================================")
	slog.Info("=== Демо 3: Очередь работников с retry ===")
	slog.Info("============================================")

	const (
		numWorkers   = 5
		lockKey      = "order:456:processing"
		lockTTL      = 3 * time.Second
		retryDelay   = 200 * time.Millisecond
		maxRetries   = 20
		workDuration = 500 * time.Millisecond
	)

	var wg sync.WaitGroup

	for i := range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			l := lock.New(rdb, lockKey, lockTTL)

			slog.Info("работник пытается захватить блокировку", "worker", i)

			acquired, acqErr := l.AcquireWithRetry(ctx, retryDelay, maxRetries)
			if acqErr != nil {
				slog.Error("ошибка при захвате", "worker", i, "error", acqErr)
				return
			}

			if !acquired {
				slog.Warn("работник не смог захватить блокировку", "worker", i)
				return
			}

			slog.Info("работник захватил блокировку — обрабатывает заказ", "worker", i)

			// Симулируем полезную работу. Блокировка удерживается —
			// другие работники ждут в AcquireWithRetry, пока мы не вызовем Release
			timer := time.NewTimer(workDuration)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}

			released, relErr := l.Release(ctx)
			if relErr != nil {
				slog.Error("ошибка при освобождении", "worker", i, "error", relErr)
				return
			}

			slog.Info("работник освободил блокировку", "worker", i, "released", released)
		}()
	}

	wg.Wait()
	slog.Info("все работники завершили работу")
	slog.Info("")
}
