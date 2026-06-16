// Package metrics инициализирует OTel Meter Provider с OTLP gRPC экспортером
// и определяет бизнес-метрики приложения
//
// Иерархия OTel Metrics:
//
//	MeterProvider — фабрика Meter'ов, управляет экспортом метрик (куда и как часто отправлять)
//	  └── Meter — именованный «набор инструментов» (обычно один на сервис/библиотеку)
//	        └── Instrument — конкретная метрика (Counter, UpDownCounter, Histogram и др.)
//
// Аналогия: MeterProvider — это электростанция, Meter — щиток в квартире,
// Instrument — конкретный счётчик (воды, света, газа)
//
// gRPC-метрики (rpc.server.duration и др.) собираются автоматически
// пакетом otelgrpc через StatsHandler — здесь только кастомные метрики
package metrics

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const (
	defaultOTLPEndpoint = "localhost:4317" // gRPC-адрес OTLP коллектора
	serviceName         = "ufo-service"
	shutdownTimeout     = 5 * time.Second
)

var (
	provider *sdkmetric.MeterProvider
	initOnce sync.Once

	// SightingsCreatedTotal — общее количество созданных наблюдений НЛО
	// Тип: Counter (монотонный счётчик) — только растёт, никогда не уменьшается
	// Применение: подсчёт событий (запросы, ошибки, созданные объекты)
	// В Prometheus: ufo_service_sightings_created_total
	SightingsCreatedTotal metric.Int64Counter

	// SightingsActive — текущее количество наблюдений в хранилище
	// Тип: UpDownCounter — растёт при Create (+1), уменьшается при Delete (-1)
	// Применение: текущее состояние (активные соединения, элементы в очереди, записи в кэше)
	// В Prometheus: ufo_service_sightings_active
	SightingsActive metric.Int64UpDownCounter

	// RPCDuration — распределение длительности обработки gRPC запросов (в секундах)
	// Тип: Histogram — записывает каждое значение, SDK автоматически считает
	// бакеты, count, sum. Позволяет строить перцентили (p50, p95, p99)
	// Применение: латентность запросов, размеры payload, длительность операций
	// В Prometheus: ufo_service_rpc_duration_seconds_bucket/count/sum
	RPCDuration metric.Float64Histogram
)

// Init создаёт OTel Meter Provider с OTLP gRPC экспортером и регистрирует бизнес-метрики
// Метрики отправляются push-моделью в OpenTelemetry Collector
func Init() {
	initOnce.Do(func() {
		ctx := context.Background()

		// OTLP gRPC экспортер — push метрик в OTel Collector
		exporter, err := otlpmetricgrpc.New(
			ctx,
			otlpmetricgrpc.WithEndpoint(otlpEndpoint()),
			otlpmetricgrpc.WithInsecure(), // без TLS для локальной разработки
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "metrics: не удалось создать OTLP экспортер: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "metrics: не удалось создать ресурс: %v\n", err)
			return
		}

		// Бакеты для гистограмм, подходящие для быстрого in-memory сервиса (единица: секунды)
		// Дефолтные бакеты OTel SDK (0, 5, 10, 25, ...) слишком крупные — все запросы <5s
		// попадают в один бакет, и histogram_quantile показывает ~5s вместо реальных <1ms
		histogramBuckets := []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5}

		// MeterProvider — корневой объект, управляющий сбором и экспортом метрик
		// PeriodicReader каждые N секунд собирает значения всех инструментов и пушит в экспортер
		provider = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
				exporter,
				// 10 секунд вместо дефолтных 60 — чтобы метрики быстрее появлялись
				// в Prometheus/Grafana при локальной разработке
				sdkmetric.WithInterval(10*time.Second),
			)),
			// View переопределяет дефолтные бакеты для автоматической гистограммы otelgrpc
			// (rpc.server.call.duration), которая собирается через StatsHandler
			sdkmetric.WithView(sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.call.duration"},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: histogramBuckets,
					},
				},
			)),
		)

		// Регистрируем провайдер глобально — otel.Meter() будет использовать его
		otel.SetMeterProvider(provider)

		createInstruments(histogramBuckets)
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

// createInstruments создаёт инструменты (метрики) через Meter
// Meter — именованный «набор инструментов», привязанный к MeterProvider
// Имя Meter'а ("ufo-service") помогает понять, какой компонент породил метрику
func createInstruments(histogramBuckets []float64) {
	meter := otel.Meter("ufo-service")

	var err error

	// Counter — монотонный счётчик, только Add(ctx, положительное_значение)
	SightingsCreatedTotal, err = meter.Int64Counter(
		"ufo_service.sightings.created",
		metric.WithDescription("Общее количество созданных наблюдений НЛО."),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "metrics: не удалось создать счётчик: %v\n", err)
	}

	// UpDownCounter — счётчик, который может расти и уменьшаться: Add(ctx, +1) / Add(ctx, -1)
	SightingsActive, err = meter.Int64UpDownCounter(
		"ufo_service.sightings.active",
		metric.WithDescription("Текущее количество наблюдений НЛО в хранилище."),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "metrics: не удалось создать up-down счётчик: %v\n", err)
	}

	// Histogram — записывает каждое значение в бакеты (buckets)
	// SDK автоматически агрегирует: count, sum и распределение по бакетам
	// Из бакетов Prometheus строит перцентили: histogram_quantile(0.95, ...)
	//
	// ВАЖНО: дефолтные бакеты OTel SDK (0, 5, 10, 25, 50, ...) рассчитаны на миллисекунды,
	// но единица метрики — секунды. Все запросы быстрее 5 сек попадают в один бакет [0, 5),
	// и histogram_quantile линейно интерполирует внутри → показывает ~5s вместо реальных <1ms
	// Поэтому задаём явные границы, подходящие для быстрого in-memory сервиса
	RPCDuration, err = meter.Float64Histogram(
		"ufo_service.rpc.duration",
		metric.WithDescription("Длительность обработки gRPC запросов (секунды)."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			histogramBuckets...,
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "metrics: не удалось создать гистограмму: %v\n", err)
	}
}

// otlpEndpoint возвращает адрес OTLP коллектора
// NB: SDK otlpmetricgrpc автоматически читает env OTEL_EXPORTER_OTLP_ENDPOINT
// и использует дефолт localhost:4317 — эта функция дублирует встроенное поведение
// Оставлена для наглядности в учебном примере
func otlpEndpoint() string {
	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
		return ep
	}

	return defaultOTLPEndpoint
}
