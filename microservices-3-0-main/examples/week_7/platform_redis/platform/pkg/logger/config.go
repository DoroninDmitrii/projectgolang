package logger

import "time"

const (
	// defaultOTLPEndpoint — gRPC-адрес OTLP коллектора по умолчанию
	defaultOTLPEndpoint = "localhost:4317"

	// defaultShutdownTimeout — таймаут на финальную отправку логов при завершении приложения
	defaultShutdownTimeout = 2 * time.Second
)

// Config — конфигурация для инициализации логгера
type Config struct {
	// Level — уровень логирования: "debug", "info", "warn", "error".
	// При невалидном значении используется "info".
	Level string
	// ServiceName — имя сервиса, прикрепляется к каждому логу через resource (service.name)
	ServiceName string
	// Environment — окружение развёртывания (например, "production", "staging", "development")
	Environment string
	// EnableOTLP — включает отправку логов в OTLP коллектор (помимо stdout)
	// При false логи пишутся только в stdout в JSON-формате
	EnableOTLP bool
	// CollectorEndpoint — адрес OTLP-коллектора (например, "localhost:4317")
	// Если не задан, используется defaultOTLPEndpoint
	CollectorEndpoint string
}
