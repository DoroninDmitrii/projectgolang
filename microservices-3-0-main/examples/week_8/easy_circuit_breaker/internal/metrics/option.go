package metrics

import (
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// options хранит настройки MeterProvider
type options struct {
	// collectorEndpoint — адрес OTLP-коллектора (например, "localhost:4317")
	// Если не задан, OTel SDK сам прочитает env-переменную OTEL_EXPORTER_OTLP_ENDPOINT,
	// а при её отсутствии упадёт на дефолт "https://localhost:4317".
	// Передавайте явно, чтобы endpoint был единой правдой из main, как у tracing/logger.
	collectorEndpoint string

	// interval — интервал экспорта метрик в коллектор
	// По умолчанию 10 секунд (вместо дефолтных 60 у OTel SDK) —
	// чтобы метрики быстрее появлялись в Prometheus/Grafana при локальной разработке
	interval time.Duration

	// views — пользовательские View для переопределения агрегации метрик
	// Например, кастомные бакеты гистограмм для конкретных инструментов
	views []sdkmetric.View
}

// Option — функциональная опция для настройки MeterProvider
type Option func(*options)

// WithCollectorEndpoint задаёт адрес OTLP-коллектора (например, "localhost:4317")
// Если опция не указана, SDK прочитает OTEL_EXPORTER_OTLP_ENDPOINT или упадёт на
// дефолт "https://localhost:4317". Используйте, чтобы endpoint метрик был согласован
// с endpoint'ом трейсера и логера — единая правда из main.
func WithCollectorEndpoint(endpoint string) Option {
	return func(o *options) {
		o.collectorEndpoint = endpoint
	}
}

// WithInterval задаёт интервал экспорта метрик
// По умолчанию 10 секунд — подходит для локальной разработки
// В production рекомендуется 15–60 секунд
func WithInterval(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.interval = d
		}
	}
}

// WithView добавляет View для переопределения агрегации конкретной метрики
//
// Пример — кастомные бакеты для гистограммы rpc.server.call.duration:
//
//	metrics.WithView(sdkmetric.NewView(
//	    sdkmetric.Instrument{Name: "rpc.server.call.duration"},
//	    sdkmetric.Stream{
//	        Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
//	            Boundaries: []float64{0.0001, 0.001, 0.01, 0.1, 0.5, 1, 5},
//	        },
//	    },
//	))
func WithView(v sdkmetric.View) Option {
	return func(o *options) {
		o.views = append(o.views, v)
	}
}
