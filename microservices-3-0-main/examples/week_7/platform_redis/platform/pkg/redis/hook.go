package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"go.opentelemetry.io/otel/trace"
)

// Compile-time проверка, что observabilityHook реализует redis.Hook
var _ redis.Hook = (*observabilityHook)(nil)

const (
	// instrumentationName — имя инструментации для OTel
	// Используется при создании Tracer и Meter, чтобы в Jaeger/Grafana
	// было видно, какая библиотека породила спан или метрику
	instrumentationName = "platform/redis"

	// maxArgsLength — максимальная длина аргументов команды в атрибуте db.statement
	// Ограничиваем, чтобы не раздувать спаны большими значениями (например, SET с JSON на 10KB)
	maxArgsLength = 256
)

// observabilityHook реализует redis.Hook — интерфейс go-redis для перехвата команд
//
// go-redis вызывает методы хука в определённом порядке:
//
//	DialHook           — при установке TCP-соединения с Redis
//	ProcessHook        — при выполнении одиночной команды (GET, SET, DEL и т.д.)
//	ProcessPipelineHook — при выполнении пайплайна (группы команд за одно обращение к серверу)
//
// Каждый метод получает next-функцию — оригинальное поведение go-redis
// Хук оборачивает её, добавляя: трейсы (спан), метрики (duration + errors) и логирование
type observabilityHook struct {
	opts options

	// Атрибуты сервера для OTel-спанов (server.address, server.port)
	serverAddr string
	serverPort string

	// Метрики — создаются один раз при создании хука, если opts.enableMetrics = true
	cmdDuration metric.Float64Histogram
	cmdErrors   metric.Int64Counter
	cacheHits   metric.Int64Counter
	cacheMisses metric.Int64Counter
}

// newObservabilityHook создаёт хук с выбранными аспектами observability
func newObservabilityHook(host, port string, opts options) (*observabilityHook, error) {
	h := &observabilityHook{
		serverAddr: host,
		serverPort: port,
		opts:       opts,
	}

	if opts.enableMetrics {
		if err := h.initMetrics(); err != nil {
			return nil, fmt.Errorf("не удалось инициализировать метрики Redis: %w", err)
		}
	}

	return h, nil
}

// initMetrics создаёт OTel-инструменты для метрик Redis
func (h *observabilityHook) initMetrics() error {
	meter := otel.Meter(instrumentationName)

	var err error

	// Histogram длительности команд — позволяет строить перцентили (p50, p95, p99)
	// Бакеты подобраны под профиль Redis-команд (0.1ms–1s), а не HTTP-запросов
	// Дефолтные бакеты OTel начинаются с 5ms — для Redis это бесполезно,
	// потому что 99% команд быстрее и все попадают в один бакет
	h.cmdDuration, err = meter.Float64Histogram(
		"redis.command.duration",
		metric.WithDescription("Длительность выполнения Redis-команд."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005,
			0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
		),
	)
	if err != nil {
		return fmt.Errorf("не удалось создать метрику redis.command.duration: %w", err)
	}

	// Counter ошибок — общее количество ошибок Redis-команд (без redis.Nil)
	// В Prometheus: redis_command_errors_total{db_operation_name="SET"}.
	h.cmdErrors, err = meter.Int64Counter(
		"redis.command.errors",
		metric.WithDescription("Количество ошибок Redis-команд."),
	)
	if err != nil {
		return fmt.Errorf("не удалось создать метрику redis.command.errors: %w", err)
	}

	// Counter попаданий в кэш — ключ найден в Redis (GET вернул значение)
	// В Prometheus: redis_cache_hits_total
	h.cacheHits, err = meter.Int64Counter(
		"redis.cache.hits",
		metric.WithDescription("Количество попаданий в кэш (ключ найден)."),
	)
	if err != nil {
		return fmt.Errorf("не удалось создать метрику redis.cache.hits: %w", err)
	}

	// Counter промахов кэша — ключ не найден (GET вернул redis.Nil)
	// В Prometheus: redis_cache_misses_total
	h.cacheMisses, err = meter.Int64Counter(
		"redis.cache.misses",
		metric.WithDescription("Количество промахов кэша (ключ не найден)."),
	)
	if err != nil {
		return fmt.Errorf("не удалось создать метрику redis.cache.misses: %w", err)
	}

	return nil
}

// DialHook оборачивает установку TCP-соединения с Redis
func (h *observabilityHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		start := time.Now()

		var span trace.Span
		if h.opts.enableTracing {
			ctx, span = otel.Tracer(instrumentationName).Start(
				ctx, "redis.dial",
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					semconv.DBSystemNameRedis,
					attribute.String("server.address", h.serverAddr),
					attribute.String("server.port", h.serverPort),
					attribute.String("network.transport", network),
				),
			)

			defer span.End()
		}

		conn, err := next(ctx, network, addr)
		duration := time.Since(start)

		if err != nil {
			if h.opts.enableTracing {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}

			if h.opts.enableLogging {
				slog.Default().ErrorContext(
					ctx, "не удалось установить соединение с Redis",
					"addr", addr,
					"duration", duration,
					"error", err,
				)
			}
		}

		return conn, err
	}
}

// ProcessHook оборачивает выполнение одиночной Redis-команды
// Это основная точка observability — здесь создаются спаны, записываются метрики
// и логируются медленные/ошибочные команды
func (h *observabilityHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		cmdName := cmd.FullName()

		// === Tracing ===
		// Создаём клиентский спан с атрибутами по OTel Semantic Conventions для БД
		// SpanKindClient — указывает, что это исходящий вызов к внешней системе
		var span trace.Span
		if h.opts.enableTracing {
			ctx, span = otel.Tracer(instrumentationName).Start(
				ctx, "redis."+cmdName,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					semconv.DBSystemNameRedis,
					attribute.String("db.operation.name", cmdName),
					attribute.String("db.statement", truncateArgs(cmd)),
					attribute.String("server.address", h.serverAddr),
					attribute.String("server.port", h.serverPort),
				),
			)

			defer span.End()
		}

		// Выполняем оригинальную команду
		err := next(ctx, cmd)

		duration := time.Since(start)

		if err != nil && !isNilErr(err) && h.opts.enableTracing {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}

		// === Metrics ===
		h.recordMetrics(ctx, cmdName, duration, err)

		// === Logging ===
		h.logCommand(ctx, cmdName, cmd, duration, err)

		return err
	}
}

// ProcessPipelineHook оборачивает выполнение пайплайна — группы команд,
// отправляемых за одно обращение к серверу
func (h *observabilityHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		cmdNames := pipelineCmdNames(cmds)

		var span trace.Span
		if h.opts.enableTracing {
			ctx, span = otel.Tracer(instrumentationName).Start(
				ctx, "redis.pipeline",
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					semconv.DBSystemNameRedis,
					attribute.String("db.operation.name", "pipeline"),
					attribute.Int("db.redis.pipeline.length", len(cmds)),
					attribute.String("db.statement", cmdNames),
					attribute.String("server.address", h.serverAddr),
					attribute.String("server.port", h.serverPort),
				),
			)

			defer span.End()
		}

		err := next(ctx, cmds)

		duration := time.Since(start)

		// В pipeline next() может вернуть nil, но отдельные команды могут содержать ошибки
		// Записываем первую найденную ошибку в спан
		if h.opts.enableTracing {
			for _, cmd := range cmds {
				if cmd.Err() != nil && !isNilErr(cmd.Err()) {
					span.RecordError(cmd.Err())
					span.SetStatus(codes.Error, cmd.Err().Error())

					break
				}
			}
		}

		h.recordMetrics(ctx, "pipeline", duration, err)
		h.logCommand(ctx, "pipeline("+cmdNames+")", nil, duration, err)

		return err
	}
}

// recordMetrics записывает длительность команды и ошибки в OTel-метрики
func (h *observabilityHook) recordMetrics(ctx context.Context, cmdName string, duration time.Duration, err error) {
	if !h.opts.enableMetrics {
		return
	}

	attrs := metric.WithAttributes(
		attribute.String("db.operation.name", cmdName),
	)

	if h.cmdDuration != nil {
		h.cmdDuration.Record(ctx, duration.Seconds(), attrs)
	}

	if err != nil && !isNilErr(err) && h.cmdErrors != nil {
		h.cmdErrors.Add(ctx, 1, attrs)
	}

	// Hit/Miss метрики — считаем только для GET-подобных команд
	// GET возвращает redis.Nil при промахе, nil при попадании
	if isReadCommand(cmdName) {
		if isNilErr(err) {
			h.cacheMisses.Add(ctx, 1, attrs)
		} else if nil == err {
			h.cacheHits.Add(ctx, 1, attrs)
		}
	}
}

// isReadCommand проверяет, является ли команда операцией чтения из кэша
func isReadCommand(cmdName string) bool {
	switch strings.ToUpper(cmdName) {
	case "GET", "MGET", "GETEX", "GETDEL", "GETRANGE", // строки
		"HGET", "HGETALL", "HMGET", "HVALS", "HKEYS": // хеш-таблицы
		return true
	default:
		return false
	}
}

// logCommand логирует ошибочные и медленные Redis-команды
func (h *observabilityHook) logCommand(ctx context.Context, cmdName string, cmd redis.Cmder, duration time.Duration, err error) {
	if !h.opts.enableLogging {
		return
	}

	if err != nil && !isNilErr(err) {
		slog.Default().ErrorContext(
			ctx, "ошибка выполнения Redis-команды",
			"cmd", cmdName,
			"duration", duration,
			"error", err,
		)

		return
	}

	if duration >= h.opts.slowThreshold {
		args := ""
		if cmd != nil {
			args = truncateArgs(cmd)
		}

		slog.Default().WarnContext(
			ctx, "медленная Redis-команда",
			"cmd", cmdName,
			"duration", duration,
			"args", args,
		)
	}
}

// truncateArgs возвращает строковое представление аргументов команды,
// обрезанное до maxArgsLength символов. Это защищает от попадания больших
// значений (JSON, бинарные данные) в спаны и логи
func truncateArgs(cmd redis.Cmder) string {
	s := cmd.String()
	if len(s) > maxArgsLength {
		return s[:maxArgsLength] + "..."
	}

	return s
}

// pipelineCmdNames возвращает строку с именами команд пайплайна через запятую
func pipelineCmdNames(cmds []redis.Cmder) string {
	names := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		names = append(names, cmd.FullName())
	}

	return strings.Join(names, ", ")
}

// isNilErr проверяет, является ли ошибка redis.Nil
// redis.Nil означает "ключ не найден" — это штатный ответ Redis, аналог SQL-ного sql.ErrNoRows
// Если считать его ошибкой, каждый cache miss будет красным спаном в Jaeger
// и +1 в счётчик ошибок — dashboard'ы станут бесполезными
func isNilErr(err error) bool {
	return errors.Is(err, redis.Nil)
}
