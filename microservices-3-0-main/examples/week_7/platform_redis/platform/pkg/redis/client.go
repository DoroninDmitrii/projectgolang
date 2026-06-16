package redis

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultDialTimeout  = 5 * time.Second
	defaultReadTimeout  = 3 * time.Second
	defaultWriteTimeout = 3 * time.Second
)

// NewClient создаёт Redis-клиент с принудительными настройками платформы,
// проверяет подключение и возвращает готовый экземпляр
//
// Платформенные гарантии (применяются поверх переданных opts):
//   - ContextTimeoutEnabled = true (go-redis уважает context deadline)
//   - Дефолтные таймауты, если не заданы (dial: 5s, read/write: 3s)
//
// Observability подключается через функциональные опции:
//
//	// Без observability — клиент как в week_6
//	rdb, err := redis.NewClient(opts)
//
//	// С полным observability — трейсы, метрики и логи из коробки
//	rdb, err := redis.NewClient(opts,
//	    redis.WithTracing(),
//	    redis.WithMetrics(),
//	    redis.WithLogging(100 * time.Millisecond),
//	)
//
// Хук использует глобальный TracerProvider, MeterProvider из пакета otel
// и глобальный логер slog.Default()
// Убедитесь, что они инициализированы до вызова NewClient
func NewClient(opts *redis.Options, observabilityOpts ...Option) (*redis.Client, error) {
	applyDefaults(opts)

	rdb := redis.NewClient(opts)

	// Подключаем observability-хук, если включён хотя бы один аспект
	// Хук реализует redis.Hook — интерфейс go-redis для перехвата команд
	// Это позволяет добавить трейсы/метрики/логи прозрачно для потребителя:
	// потребитель работает со стандартным *redis.Client, observability работает автоматически
	o := &options{}
	for _, opt := range observabilityOpts {
		opt(o)
	}

	if o.hasAny() {
		host, port, _ := net.SplitHostPort(opts.Addr) //nolint:gosec // Если Addr невалидный — host будет пустым, но Ping ниже всё равно упадёт

		hook, err := newObservabilityHook(host, port, *o)
		if err != nil {
			_ = rdb.Close() //nolint:gosec // Закрываем клиент при ошибке инициализации хука

			return nil, err
		}

		rdb.AddHook(hook)
	}

	err := rdb.Ping(context.Background()).Err()
	if err != nil {
		_ = rdb.Close() //nolint:gosec // Закрываем клиент при ошибке Ping, ошибку Close игнорируем

		return nil, fmt.Errorf("не удалось подключиться к Redis: %w", err)
	}

	slog.Info("подключение к Redis установлено", "address", opts.Addr)

	return rdb, nil
}

// applyDefaults выставляет принудительные платформенные настройки
// и дефолтные таймауты, если они не были заданы вызывающим кодом
func applyDefaults(opts *redis.Options) {
	// Принудительно включаем поддержку context deadline в go-redis
	//
	// По умолчанию go-redis ИГНОРИРУЕТ дедлайн контекста — команда будет ждать
	// до DialTimeout/ReadTimeout/WriteTimeout, даже если ctx уже истёк
	// С этим флагом go-redis прервёт команду, как только ctx.Done() закроется
	//
	// Это критично для микросервисов: контекст приходит из gRPC/HTTP с дедлайном запроса
	// Без этого флага Redis-вызов может пережить дедлайн, и горутина будет ждать
	// ответа от Redis, который уже никому не нужен
	//
	// DialTimeout/ReadTimeout/WriteTimeout при этом остаются верхней границей —
	// ContextTimeoutEnabled позволяет сузить их через контекст на уровне конкретного вызова
	opts.ContextTimeoutEnabled = true

	if opts.DialTimeout == 0 {
		opts.DialTimeout = defaultDialTimeout
	}

	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = defaultReadTimeout
	}

	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = defaultWriteTimeout
	}
}
