package tests

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_lock/internal/lock"
)

var rdb *redis.Client

// TestMain поднимает Redis через testcontainers
func TestMain(m *testing.M) {
	os.Exit(setupAndRun(m))
}

func setupAndRun(m *testing.M) int {
	_ = godotenv.Load("../.env")

	ctx := context.Background()

	// Поднимаем Redis через testcontainers
	// Образ должен совпадать с docker-compose.yml
	redisContainer, err := tcredis.Run(ctx, os.Getenv("REDIS_IMAGE"))
	if err != nil {
		slog.Error("не удалось поднять Redis контейнер", "error", err)
		return 1
	}
	defer func() { _ = redisContainer.Terminate(ctx) }()

	redisAddr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		slog.Error("не удалось получить адрес Redis", "error", err)
		return 1
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	if pingErr := rdb.Ping(ctx).Err(); pingErr != nil {
		slog.Error("не удалось подключиться к Redis", "error", pingErr)
		return 1
	}

	slog.Info("Redis контейнер запущен", "addr", redisAddr)

	return m.Run()
}

// TestLock_AcquireAndRelease проверяет базовый цикл: захват → освобождение
func TestLock_AcquireAndRelease(t *testing.T) {
	ctx := context.Background()
	l := lock.New(rdb, "test:acquire-release", 10*time.Second)

	// Захватываем блокировку
	acquired, err := l.Acquire(ctx)
	require.NoError(t, err)
	assert.True(t, acquired, "блокировка должна быть захвачена")

	// Освобождаем блокировку
	released, err := l.Release(ctx)
	require.NoError(t, err)
	assert.True(t, released, "блокировка должна быть освобождена")

	// Повторный Release должен вернуть false (ключ уже удалён)
	released, err = l.Release(ctx)
	require.NoError(t, err)
	assert.False(t, released, "повторное освобождение должно вернуть false")
}

// TestLock_MutualExclusion проверяет, что второй Acquire возвращает false
func TestLock_MutualExclusion(t *testing.T) {
	ctx := context.Background()
	key := "test:mutual-exclusion"

	l1 := lock.New(rdb, key, 10*time.Second)
	l2 := lock.New(rdb, key, 10*time.Second)

	// Первый захватывает
	acquired, err := l1.Acquire(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Второй не может захватить
	acquired, err = l2.Acquire(ctx)
	require.NoError(t, err)
	assert.False(t, acquired, "второй Acquire должен вернуть false — блокировка занята")

	// Освобождаем первую блокировку
	_, err = l1.Release(ctx)
	require.NoError(t, err)

	// Теперь второй может захватить
	acquired, err = l2.Acquire(ctx)
	require.NoError(t, err)
	assert.True(t, acquired, "после освобождения второй должен захватить блокировку")

	_, _ = l2.Release(ctx)
}

// TestLock_SafeRelease проверяет, что чужой UUID не может освободить блокировку
func TestLock_SafeRelease(t *testing.T) {
	ctx := context.Background()
	key := "test:safe-release"

	// Захватываем блокировку с одним UUID
	l1 := lock.New(rdb, key, 10*time.Second)
	acquired, err := l1.Acquire(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Пытаемся освободить с другим UUID — должно вернуть false
	l2 := lock.NewWithValue(rdb, key, "wrong-uuid", 10*time.Second)
	released, err := l2.Release(ctx)
	require.NoError(t, err)
	assert.False(t, released, "чужой UUID не должен освободить блокировку")

	// Оригинальный владелец может освободить
	released, err = l1.Release(ctx)
	require.NoError(t, err)
	assert.True(t, released, "владелец должен освободить блокировку")
}

// TestLock_TTLExpiry проверяет, что ключ пропадает после TTL
func TestLock_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	key := "test:ttl-expiry"
	ttl := 1 * time.Second

	l := lock.New(rdb, key, ttl)
	acquired, err := l.Acquire(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Проверяем, что ключ существует
	exists, err := rdb.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists, "ключ должен существовать")

	// Ждём, пока TTL истечёт
	timer := time.NewTimer(ttl + 500*time.Millisecond)
	<-timer.C

	// Ключ должен исчезнуть
	exists, err = rdb.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "ключ должен исчезнуть после TTL")
}

// TestLock_AcquireWithRetry проверяет, что retry успешен после освобождения
func TestLock_AcquireWithRetry(t *testing.T) {
	ctx := context.Background()
	key := "test:retry"

	// Захватываем блокировку
	l1 := lock.New(rdb, key, 10*time.Second)
	acquired, err := l1.Acquire(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)

	// В отдельной горутине через 300ms освобождаем
	go func() {
		timer := time.NewTimer(300 * time.Millisecond)
		<-timer.C

		_, _ = l1.Release(context.Background())
	}()

	// Второй клиент пытается с retry
	l2 := lock.New(rdb, key, 10*time.Second)
	acquired, err = l2.AcquireWithRetry(ctx, 100*time.Millisecond, 10)
	require.NoError(t, err)
	assert.True(t, acquired, "retry должен успешно захватить блокировку после освобождения")

	_, _ = l2.Release(ctx)
}

// TestLock_ContextCancellation проверяет, что отмена контекста прерывает retry
func TestLock_ContextCancellation(t *testing.T) {
	key := "test:ctx-cancel"

	// Захватываем блокировку, чтобы retry не мог завершиться
	l1 := lock.New(rdb, key, 30*time.Second)
	acquired, err := l1.Acquire(context.Background())
	require.NoError(t, err)
	assert.True(t, acquired)
	defer func() { _, _ = l1.Release(context.Background()) }()

	// Контекст с отменой через 200ms
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	l2 := lock.New(rdb, key, 10*time.Second)

	var (
		retryAcquired bool
		retryErr      error
		wg            sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		retryAcquired, retryErr = l2.AcquireWithRetry(ctx, 50*time.Millisecond, 100)
	}()

	wg.Wait()

	assert.False(t, retryAcquired, "после отмены контекста блокировка не должна быть захвачена")
	assert.ErrorIs(t, retryErr, context.DeadlineExceeded, "ошибка должна быть context.DeadlineExceeded")
}

// TestOnceExecutor_SingleExecution проверяет, что fn выполняется ровно один раз
func TestOnceExecutor_SingleExecution(t *testing.T) {
	ctx := context.Background()
	lockKey := "test:once:lock"
	doneKey := "test:once:done"
	defer rdb.Del(ctx, lockKey, doneKey)

	once := lock.NewOnceExecutor(rdb, lockKey, 10*time.Second, doneKey, 10*time.Second)

	var count atomic.Int64

	const numWorkers = 10

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := once.Do(ctx, func(_ context.Context) error {
				count.Add(1)
				return nil
			})
			// Допустимые результаты: nil (выполнил), ErrAlreadyDone или ErrLockBusy
			if err != nil {
				assert.True(
					t,
					errors.Is(err, lock.ErrAlreadyDone) || errors.Is(err, lock.ErrLockBusy),
					"неожиданная ошибка: %v", err,
				)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(1), count.Load(), "fn должна быть вызвана ровно 1 раз")

	// Idempotency key должен существовать
	exists, err := rdb.Exists(ctx, doneKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists, "idempotency key должен существовать после выполнения")
}

// TestOnceExecutor_AlreadyDone проверяет, что повторный вызов возвращает ErrAlreadyDone
func TestOnceExecutor_AlreadyDone(t *testing.T) {
	ctx := context.Background()
	lockKey := "test:once-done:lock"
	doneKey := "test:once-done:done"
	defer rdb.Del(ctx, lockKey, doneKey)

	once := lock.NewOnceExecutor(rdb, lockKey, 10*time.Second, doneKey, 10*time.Second)

	// Первый вызов — успешный
	err := once.Do(ctx, func(_ context.Context) error {
		return nil
	})
	require.NoError(t, err)

	// Второй вызов — ErrAlreadyDone
	err = once.Do(ctx, func(_ context.Context) error {
		t.Fatal("fn не должна вызываться повторно")
		return nil
	})
	assert.ErrorIs(t, err, lock.ErrAlreadyDone)
}

// TestOnceExecutor_ErrorDoesNotSetDoneKey проверяет, что при ошибке fn
// idempotency key НЕ ставится — повторная попытка возможна
func TestOnceExecutor_ErrorDoesNotSetDoneKey(t *testing.T) {
	ctx := context.Background()
	lockKey := "test:once-err:lock"
	doneKey := "test:once-err:done"
	defer rdb.Del(ctx, lockKey, doneKey)

	once := lock.NewOnceExecutor(rdb, lockKey, 10*time.Second, doneKey, 10*time.Second)

	errBoom := assert.AnError

	// Первый вызов — fn возвращает ошибку
	err := once.Do(ctx, func(_ context.Context) error {
		return errBoom
	})
	assert.ErrorIs(t, err, errBoom)

	// Idempotency key НЕ должен быть установлен
	exists, err := rdb.Exists(ctx, doneKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "idempotency key не должен быть установлен при ошибке")

	// Повторный вызов — должен выполниться
	err = once.Do(ctx, func(_ context.Context) error {
		return nil
	})
	require.NoError(t, err)
}
