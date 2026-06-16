package redis

import (
	"context"
	"fmt"
	"log/slog"
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
// Логгер — *slog.Logger из стандартной библиотеки (log/slog)
// slog — это стандартный фасад логирования в Go (с 1.21+), аналог io.Writer для I/O
// При смене бэкенда (zap, logrus и т.д.) достаточно подменить slog.Handler при инициализации,
// сигнатуры потребителей не меняются
func NewClient(opts *redis.Options, logger *slog.Logger) (*redis.Client, error) {
	applyDefaults(opts)

	rdb := redis.NewClient(opts)

	err := rdb.Ping(context.Background()).Err()
	if err != nil {
		_ = rdb.Close() //nolint:gosec // Закрываем клиент при ошибке Ping, ошибку Close игнорируем

		return nil, fmt.Errorf("не удалось подключиться к Redis: %w", err)
	}

	logger.Info("подключение к Redis установлено", "address", opts.Addr)

	return rdb, nil
}

// applyDefaults выставляет принудительные платформенные настройки
// и дефолтные таймауты, если они не были заданы вызывающим кодом
func applyDefaults(opts *redis.Options) {
	// Принудительно: go-redis уважает context deadline при выполнении команд
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
