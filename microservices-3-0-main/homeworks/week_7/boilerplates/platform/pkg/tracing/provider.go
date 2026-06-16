package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

// InitTracer инициализирует глобальный TracerProvider и возвращает функцию
// для корректного завершения (flush + shutdown). Вызовите её при остановке приложения
//
// Пример использования:
//
//	shutdown, err := tracing.InitTracer(ctx, cfg)
//	if err != nil { ... }
//	defer shutdown(ctx)
func InitTracer(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	samplingRatio := cfg.SamplingRatio
	if samplingRatio == 0 {
		samplingRatio = defaultSamplingRatio
	}

	// OTLP gRPC экспортер — отправляет спаны в коллектор (Jaeger, Tempo, OTEL Collector и т.д.)
	exporter, err := otlptracegrpc.New(
		ctx,
		// Адрес OTLP-коллектора (например, "localhost:4317")
		otlptracegrpc.WithEndpoint(cfg.CollectorEndpoint),
		// TLS отключён — подходит для локальной разработки и sidecar-коллекторов
		// В проде используйте WithTLSCredentials
		otlptracegrpc.WithInsecure(),
		// Таймаут на установку соединения с коллектором
		otlptracegrpc.WithTimeout(defaultTimeout),
		// Сжатие gzip снижает объём трафика к коллектору
		otlptracegrpc.WithCompressor(defaultCompressor),
		// Retry с экспоненциальным backoff при сбоях отправки спанов
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: defaultRetryInitInterval, // начальная задержка между попытками
			MaxInterval:     defaultRetryMaxInterval,  // потолок экспоненциального роста
			MaxElapsedTime:  defaultRetryMaxElapsed,   // общее время, после которого retry прекращаются
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	// Ресурс описывает сервис, от которого приходят трейсы
	// Эти атрибуты добавляются к каждому спану и видны в UI трейсинга
	res, err := resource.New(
		ctx,
		// Явные атрибуты сервиса — по ним ищут и фильтруют трейсы в UI
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),               // имя сервиса (обязательный атрибут)
			semconv.ServiceVersion(cfg.ServiceVersion),         // версия для корреляции с релизами
			semconv.DeploymentEnvironmentName(cfg.Environment), // окружение: production, staging, development
		),
		// Детекторы автоматически собирают информацию об инфраструктуре
		// Полезно для отладки: на каком хосте/контейнере произошла проблема
		resource.WithHost(),         // имя хоста и его идентификатор
		resource.WithOS(),           // тип и версия ОС
		resource.WithProcess(),      // PID, команда запуска, версия Go runtime
		resource.WithContainer(),    // container ID (если запущен в Docker/K8s)
		resource.WithTelemetrySDK(), // версия OpenTelemetry SDK
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		// BatchSpanProcessor — буферизирует спаны и отправляет пачками,
		// снижая нагрузку на сеть по сравнению с SimpleSpanProcessor
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		// ParentBased — если родительский спан семплирован, дочерний тоже будет
		// TraceIDRatioBased задаёт долю трейсов: 1.0 = 100% (разработка), ~0.1 = 10% (прод)
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio))),
	)

	otel.SetTracerProvider(tp)

	// Propagator определяет, КАК trace ID передаётся между сервисами
	// Когда сервис A вызывает сервис B, propagator:
	//   1. На стороне A (inject) — записывает trace ID в HTTP/gRPC заголовки запроса
	//   2. На стороне B (extract) — достаёт trace ID из заголовков и продолжает тот же трейс
	// Без этого каждый сервис создавал бы свой независимый трейс,
	// и мы не смогли бы видеть сквозную цепочку вызовов
	// Оба propagator'а — stateless-стратегии (реализации интерфейса TextMapPropagator)
	// Пустые struct'ы, потому что формат заголовков фиксирован стандартом — настраивать нечего
	// Они лишь определяют алгоритм записи/чтения trace-контекста в/из заголовков
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		// W3C TraceContext — стандартные заголовки traceparent/tracestate
		// Поддерживается Jaeger, Tempo, Datadog и другими системами трейсинга
		propagation.TraceContext{},
		// Baggage — произвольные key-value пары (user_id, tenant_id),
		// которые автоматически прокидываются через всю цепочку вызовов
		propagation.Baggage{},
	))

	// Возвращаем shutdown-функцию, которая сбросит оставшиеся спаны и освободит ресурсы
	return tp.Shutdown, nil
}
