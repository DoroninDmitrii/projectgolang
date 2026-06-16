package lock

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Lock представляет распределённую блокировку на основе Redis
type Lock struct {
	client *redis.Client
	key    string        // Ключ блокировки в Redis
	value  string        // UUID — уникальный идентификатор владельца блокировки
	ttl    time.Duration // Время жизни блокировки (автоматическое освобождение при краше)
}

// New создаёт новую блокировку с уникальным UUID
func New(client *redis.Client, key string, ttl time.Duration) *Lock {
	return &Lock{
		client: client,
		key:    key,
		value:  uuid.New().String(),
		ttl:    ttl,
	}
}

// NewWithValue создаёт блокировку с заданным значением (для тестирования)
func NewWithValue(client *redis.Client, key, value string, ttl time.Duration) *Lock {
	return &Lock{
		client: client,
		key:    key,
		value:  value,
		ttl:    ttl,
	}
}

// Acquire пытается захватить блокировку
// Использует SET key value NX EX ttl — атомарную операцию Redis:
//   - NX = Not eXists — установить только если ключ НЕ существует (mutual exclusion)
//   - EX = EXpire — задать TTL в секундах для автоматического освобождения (защита от deadlock при краше)
//
// Возвращает true, если блокировка успешно захвачена
func (l *Lock) Acquire(ctx context.Context) (bool, error) {
	// SetArgs(key, value, {Mode: "NX", TTL: ttl}) — выполняет команду SET key value NX EX ttl
	//   key   = l.key   — ключ блокировки (например, "order:123:processing")
	//   value = l.value — UUID владельца (чтобы только он мог освободить)
	//   ttl   = l.ttl   — время жизни ключа (автоосвобождение при краше процесса)
	// Возвращает "OK", если ключ был установлен (блокировка захвачена),
	// redis.Nil — если ключ уже существует (блокировка занята другим владельцем)
	err := l.client.SetArgs(ctx, l.key, l.value, redis.SetArgs{
		Mode: "NX",
		TTL:  l.ttl,
	}).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// Release освобождает блокировку, если она принадлежит текущему владельцу
//
// Использует команду DELEX key IFEQ value (Redis >= 8.0):
// удаляет ключ только если его значение совпадает с UUID владельца
// Одна атомарная команда — проще и быстрее, чем Lua-скрипт
// https://redis.io/docs/latest/commands/delex/
//
// Для Redis < 8.0 можно использовать Lua-скрипт с GET + DEL,
// выполняемый атомарно на сервере Redis (см. документацию)
// https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/
//
// Возвращает true, если блокировка была успешно освобождена
func (l *Lock) Release(ctx context.Context) (bool, error) {
	// DELEX = DEL EXtended (расширенный DEL с условиями)
	// IFEQ  = IF EQual (удалить только если значение совпадает)
	// Итого: атомарно удаляет ключ, если его значение равно value
	// Возвращает 1 (удалил) или 0 (владелец не совпал / ключ не существует)
	result, err := l.client.Do(ctx, "DELEX", l.key, "IFEQ", l.value).Int64()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// AcquireWithRetry пытается захватить блокировку с повторными попытками
// Между попытками ожидает interval. Максимальное количество попыток — maxRetries
//
// Зачем нужен retry, если можно просто уйти?
// Пример: пользователю одновременно приходят 5 разных переводов от разных отправителей
// 5 воркеров разобрали эти переводы из очереди. Каждый должен зачислить свой перевод
// на баланс одного и того же пользователя. Блокировка на ключ "user:42:balance" гарантирует,
// что записи не потеряются (read-modify-write без race condition)
// Воркер не может уйти — у него есть конкретный перевод, который он обязан зачислить
// Он ждёт, пока предыдущий воркер закончит свой перевод, захватывает блокировку и зачисляет свой
//
// Используется time.NewTimer + select вместо time.Sleep для корректной отмены через контекст
func (l *Lock) AcquireWithRetry(ctx context.Context, interval time.Duration, maxRetries int) (bool, error) {
	// maxRetries + 1: первая попытка сразу, остальные — после ожидания interval
	for i := range maxRetries + 1 {
		acquired, err := l.Acquire(ctx)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}

		slog.Info("блокировка занята, повторная попытка...", "key", l.key)

		// После последней неудачной попытки не ждём — сразу выходим
		if i == maxRetries {
			break
		}

		timer := time.NewTimer(interval)

		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}

	return false, nil
}

// Key возвращает ключ блокировки
func (l *Lock) Key() string {
	return l.key
}

// Value возвращает UUID владельца блокировки
func (l *Lock) Value() string {
	return l.value
}

// MustAcquire захватывает блокировку или возвращает ошибку
func (l *Lock) MustAcquire(ctx context.Context) error {
	acquired, err := l.Acquire(ctx)
	if err != nil {
		return err
	}
	if !acquired {
		return errors.New("не удалось захватить блокировку: ключ уже занят")
	}

	return nil
}
