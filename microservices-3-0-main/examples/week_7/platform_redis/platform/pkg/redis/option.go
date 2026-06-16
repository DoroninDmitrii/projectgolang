package redis

import "time"

const defaultSlowThreshold = 100 * time.Millisecond

// options хранит настройки observability, выбранные потребителем
// Каждый аспект (трейсы, метрики, логирование) опционален и включается
// соответствующей Option-функцией при вызове NewClient
type options struct {
	enableTracing bool
	enableMetrics bool
	enableLogging bool
	slowThreshold time.Duration
}

// Option — функциональная опция для настройки observability Redis-клиента
type Option func(*options)

// WithTracing включает создание OpenTelemetry-спанов для каждой Redis-команды
// Каждая команда порождает клиентский спан с атрибутами: db.system, db.operation.name,
// db.statement, server.address, server.port — по стандарту OpenTelemetry Semantic Conventions
func WithTracing() Option {
	return func(o *options) {
		o.enableTracing = true
	}
}

// WithMetrics включает сбор метрик через OpenTelemetry Metrics API:
//   - redis.command.duration (Histogram) — длительность выполнения команд в секундах
//   - redis.command.errors (Counter) — количество ошибок
//
// Метрики автоматически экспортируются через глобальный MeterProvider (Prometheus, OTLP и т.д.)
func WithMetrics() Option {
	return func(o *options) {
		o.enableMetrics = true
	}
}

// WithLogging включает логирование Redis-команд через slog:
//   - Ошибки — логируются всегда (уровень ERROR)
//   - Медленные команды — логируются, если длительность >= slowThreshold (уровень WARN)
//
// slowThreshold — порог, после которого команда считается медленной
// Рекомендуемые значения: 100ms для большинства приложений, 10ms для highload
func WithLogging(slowThreshold time.Duration) Option {
	return func(o *options) {
		o.enableLogging = true
		o.slowThreshold = slowThreshold

		if o.slowThreshold <= 0 {
			o.slowThreshold = defaultSlowThreshold
		}
	}
}

// hasAny возвращает true, если включён хотя бы один аспект observability
func (o *options) hasAny() bool {
	return o.enableTracing || o.enableMetrics || o.enableLogging
}
