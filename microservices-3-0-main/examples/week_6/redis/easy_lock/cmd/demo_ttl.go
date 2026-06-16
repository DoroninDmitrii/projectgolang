package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_lock/internal/lock"
)

// demoTTLProtection показывает защиту от deadlock: если процесс «упал»
// и не вызвал Release, Redis сам освободит блокировку после TTL
//
// Без TTL упавший процесс навсегда заблокирует ресурс (deadlock)
// С TTL — блокировка автоматически истечёт, и другие процессы смогут продолжить
func demoTTLProtection(ctx context.Context, rdb *redis.Client) {
	slog.Info("==========================================")
	slog.Info("=== Демо 4: TTL-защита от deadlock ===")
	slog.Info("==========================================")

	const (
		lockKey = "order:789:processing"
		lockTTL = 2 * time.Second
	)

	// Первый процесс захватывает блокировку и «падает» (не вызывает Release)
	l1 := lock.New(rdb, lockKey, lockTTL)

	acquired, err := l1.Acquire(ctx)
	if err != nil {
		slog.Error("ошибка при захвате блокировки", "error", err)
		return
	}

	slog.Info("процесс 1 захватил блокировку", "acquired", acquired)
	slog.Warn("процесс 1 «упал» — Release НЕ вызван!")
	slog.Info("без TTL это был бы deadlock навсегда...")

	// Второй процесс пытается захватить — не может (ключ ещё существует)
	l2 := lock.New(rdb, lockKey, lockTTL)

	acquired, err = l2.Acquire(ctx)
	if err != nil {
		slog.Error("ошибка при захвате блокировки", "error", err)
		return
	}

	slog.Info("процесс 2 попытался захватить (до TTL)", "acquired", acquired)

	// Ждём, пока TTL истечёт — Redis автоматически удалит ключ
	slog.Info("ожидаем истечения TTL...", "ttl", lockTTL)

	timer := time.NewTimer(lockTTL + 500*time.Millisecond)
	select {
	case <-ctx.Done():
		timer.Stop()
		return
	case <-timer.C:
	}

	// Теперь блокировка свободна — Redis сам удалил просроченный ключ
	acquired, err = l2.Acquire(ctx)
	if err != nil {
		slog.Error("ошибка при захвате блокировки", "error", err)
		return
	}

	slog.Info("процесс 2 захватил блокировку (после TTL)", "acquired", acquired)

	// Корректно освобождаем
	if _, relErr := l2.Release(ctx); relErr != nil {
		slog.Error("ошибка при освобождении блокировки", "error", relErr)
		return
	}

	slog.Info("TTL-защита работает: Redis автоматически освободил блокировку «упавшего» процесса")
}
