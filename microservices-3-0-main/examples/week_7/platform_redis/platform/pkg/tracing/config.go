package tracing

import "time"

const (
	// defaultCompressor — алгоритм сжатия для OTLP экспортера
	defaultCompressor = "gzip"

	// defaultTimeout — таймаут подключения к коллектору
	defaultTimeout = 5 * time.Second

	// Параметры retry-политики экспортера при сбоях отправки
	defaultRetryInitInterval = 500 * time.Millisecond // начальный интервал между попытками
	defaultRetryMaxInterval  = 5 * time.Second        // максимальный интервал (экспоненциальный backoff)
	defaultRetryMaxElapsed   = 30 * time.Second       // максимальное общее время retry

	// defaultSamplingRatio — доля трейсов для семплирования (1.0 = 100%)
	defaultSamplingRatio = 1.0

	// TraceIDHeader — заголовок для передачи trace ID клиенту в gRPC ответе,
	// чтобы можно было найти трейс в Jaeger/Tempo
	TraceIDHeader = "x-trace-id"
)

// Config — конфигурация для инициализации трейсера
type Config struct {
	// CollectorEndpoint — адрес OTLP-коллектора (например, "localhost:4317")
	CollectorEndpoint string
	// ServiceName — имя сервиса, отображаемое в трейсах
	ServiceName string
	// Environment — окружение развёртывания (например, "production", "staging")
	Environment string
	// ServiceVersion — версия сервиса (например, "1.0.0")
	ServiceVersion string
	// SamplingRatio — доля трейсов для семплирования: 0.0–1.0.
	// 1.0 = все трейсы (разработка), 0.1 = 10% (продакшен)
	// Если не задано (0), используется defaultSamplingRatio (100%)
	SamplingRatio float64
}
