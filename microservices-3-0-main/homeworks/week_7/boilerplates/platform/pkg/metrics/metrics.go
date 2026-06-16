// Package metrics инициализирует OTel MeterProvider с OTLP gRPC экспортером
//
// Пакет предоставляет платформенную инициализацию метрик — создание MeterProvider,
// подключение к OTel Collector и регистрацию глобального провайдера
// Бизнес-метрики (Counter, Histogram и т.д.) каждый сервис создаёт самостоятельно
//
// Иерархия OTel Metrics:
//
//	MeterProvider — фабрика Meter'ов, управляет экспортом метрик (куда и как часто отправлять)
//	  └── Meter — именованный «набор инструментов» (обычно один на сервис/библиотеку)
//	        └── Instrument — конкретная метрика (Counter, UpDownCounter, Histogram и др.)
//
// Аналогия: MeterProvider — это электростанция, Meter — щиток в квартире,
// Instrument — конкретный счётчик (воды, света, газа)
package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

const (
	defaultInterval = 10 * time.Second
	shutdownTimeout = 5 * time.Second
)

var (
	provider *sdkmetric.MeterProvider
	initOnce sync.Once
)

// Init создаёт OTel MeterProvider с OTLP gRPC экспортером и регистрирует его глобально
// Метрики отправляются push-моделью в OpenTelemetry Collector
//
// serviceName — имя сервиса, прикрепляется к каждой метрике через resource
// Функциональные опции позволяют настроить поведение (интервал, бакеты, views)
//
// После вызова Init все Meter'ы, созданные через otel.Meter(), используют этот провайдер —
// включая платформенные библиотеки (например, platform/redis)
func Init(serviceName string, opts ...Option) {
	initOnce.Do(func() {
		ctx := context.Background()

		o := &options{
			interval: defaultInterval,
		}
		for _, opt := range opts {
			opt(o)
		}

		// OTLP gRPC экспортер — push метрик в OTel Collector
		//
		// SDK автоматически читает env-переменные:
		//   OTEL_EXPORTER_OTLP_ENDPOINT (дефолт: https://localhost:4317)
		//   OTEL_EXPORTER_OTLP_INSECURE ("true" для отключения TLS)
		//   OTEL_EXPORTER_OTLP_METRICS_ENDPOINT (приоритет над общим)
		//
		// WithInsecure отключает TLS — для локальной разработки, где коллектор
		// доступен по http://localhost:4317 без сертификатов
		exporter, err := otlpmetricgrpc.New(
			ctx,
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			slog.Error("metrics: failed to create OTLP exporter", "error", err)
			return
		}

		// Resource — метаданные сервиса, прикрепляются к каждой метрике
		res, err := resource.New(
			ctx,
			resource.WithAttributes(
				semconv.ServiceName(serviceName),
				attribute.String("deployment.environment", "dev"),
			),
		)
		if err != nil {
			slog.Error("metrics: failed to create resource", "error", err)
			return
		}

		// MeterProvider — корневой объект, управляющий сбором и экспортом метрик
		// PeriodicReader каждые N секунд собирает значения всех инструментов и пушит в экспортер
		providerOpts := []sdkmetric.Option{
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
				exporter,
				sdkmetric.WithInterval(o.interval),
			)),
		}

		// Добавляем пользовательские Views, если заданы
		for _, view := range o.views {
			providerOpts = append(providerOpts, sdkmetric.WithView(view))
		}

		provider = sdkmetric.NewMeterProvider(providerOpts...)

		// Регистрируем провайдер глобально — otel.Meter() будет использовать его
		otel.SetMeterProvider(provider)
	})
}

// Flush принудительно отправляет накопленные метрики (полезно в тестах перед проверкой)
func Flush() error {
	if provider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return provider.ForceFlush(ctx)
}

// Close завершает MeterProvider, отправляя накопленные метрики
func Close() error {
	if provider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return provider.Shutdown(ctx)
}
