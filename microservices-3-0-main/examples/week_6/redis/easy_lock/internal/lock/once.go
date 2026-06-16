package lock

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrAlreadyDone означает, что операция уже была выполнена (idempotency key существует)
	ErrAlreadyDone = errors.New("операция уже выполнена")

	// ErrLockBusy означает, что блокировка занята другим процессом
	ErrLockBusy = errors.New("блокировка занята")
)

// onceExecutor выполняет действие ровно один раз среди всех конкурирующих процессов
// Аналог sync.Once, но распределённый — работает через Redis
//
// Внутри использует два механизма:
//  1. Distributed Lock — защита от одновременного выполнения (мьютекс)
//  2. Idempotency Key — защита от повторного выполнения (маркер «уже сделано»)
//
// Паттерн double-checked locking:
//   - Сначала проверяем idempotency key (быстрый путь без захвата лока)
//   - Захватываем лок
//   - Повторно проверяем idempotency key (double-check)
//   - Выполняем действие
//   - Ставим idempotency key
//   - Освобождаем лок
type onceExecutor struct {
	client  *redis.Client
	lockKey string        // Ключ блокировки (мьютекс)
	lockTTL time.Duration // TTL блокировки — страховка от deadlock при крэше
	doneKey string        // Ключ идемпотентности — маркер «уже выполнено»
	doneTTL time.Duration // TTL маркера — сколько помним факт выполнения
}

// NewOnceExecutor создаёт новый onceExecutor
//
//   - lockKey — ключ блокировки (мьютекс), например "order:123:lock".
//   - lockTTL — TTL блокировки, страховка от deadlock при крэше
//   - doneKey — ключ идемпотентности (маркер «уже выполнено»), например "order:123:done".
//   - doneTTL — TTL маркера, сколько помним факт выполнения
func NewOnceExecutor(
	client *redis.Client,
	lockKey string, lockTTL time.Duration,
	doneKey string, doneTTL time.Duration,
) *onceExecutor {
	return &onceExecutor{
		client:  client,
		lockKey: lockKey,
		lockTTL: lockTTL,
		doneKey: doneKey,
		doneTTL: doneTTL,
	}
}

func (o *onceExecutor) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	// Быстрый путь: проверяем idempotency key без захвата лока
	if o.alreadyDone(ctx) {
		return ErrAlreadyDone
	}

	// Захватываем лок. Каждый вызов Do создаёт свой Lock с уникальным UUID,
	// чтобы Release мог безопасно проверить владельца через DELEX IFEQ
	l := New(o.client, o.lockKey, o.lockTTL)

	acquired, err := l.Acquire(ctx)
	if err != nil {
		return err
	}

	if !acquired {
		return ErrLockBusy
	}

	// Double-check после захвата лока: между первой проверкой и Acquire
	// другой процесс мог успеть выполнить операцию
	if o.alreadyDone(ctx) {
		if _, relErr := l.Release(ctx); relErr != nil {
			slog.Error("ошибка при освобождении блокировки", "error", relErr)
		}
		return ErrAlreadyDone
	}

	// Выполняем бизнес-логику
	if err = fn(ctx); err != nil {
		// При ошибке освобождаем лок, но НЕ ставим idempotency key —
		// другой процесс сможет повторить попытку
		if _, relErr := l.Release(ctx); relErr != nil {
			slog.Error("ошибка при освобождении блокировки", "error", relErr)
		}
		return err
	}

	// Ставим idempotency key — операция выполнена
	// Храним timestamp — полезно для дебага:
	// можно посмотреть в Redis, когда именно операция была выполнена
	doneValue := time.Now().UTC().Format(time.RFC3339)
	if err = o.client.Set(ctx, o.doneKey, doneValue, o.doneTTL).Err(); err != nil {
		slog.Error("ошибка установки idempotency key", "key", o.doneKey, "error", err)
	}

	// Освобождаем лок — best practice по документации Redis:
	// https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/
	if _, relErr := l.Release(ctx); relErr != nil {
		slog.Error("ошибка при освобождении блокировки", "error", relErr)
	}

	return nil
}

// alreadyDone проверяет, существует ли idempotency key
func (o *onceExecutor) alreadyDone(ctx context.Context) bool {
	exists, err := o.client.Exists(ctx, o.doneKey).Result()
	if err != nil {
		slog.Error("ошибка проверки idempotency key", "key", o.doneKey, "error", err)
		return false
	}

	return exists == 1
}
