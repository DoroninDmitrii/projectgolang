// Package logger — dual-write логгер: stdout + OTLP коллектор
// Подробное описание архитектуры см. в README.md (секция "Архитектура логгера")
package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otelLogSdk "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const (
	defaultOTLPEndpoint = "localhost:4317" // gRPC-адрес OTLP коллектора (например, Alloy или OTEL Collector)
	serviceName         = "ufo-service"    // имя сервиса в метаданных логов (service.name)
	serviceEnvironment  = "dev"            // окружение (deployment.environment)
	shutdownTimeout     = 2 * time.Second  // таймаут на финальную отправку логов при завершении
)

var (
	// otelProvider хранится для корректного завершения (Close)
	otelProvider *otelLogSdk.LoggerProvider

	// initOnce гарантирует, что Init вызовется только один раз (потокобезопасность)
	initOnce sync.Once
)

// Init создаёт глобальный slog-логгер и устанавливает его через slog.SetDefault
// При enableOTLP=true логи дополнительно отправляются в OTLP коллектор
func Init(logLevel string, enableOTLP bool) {
	initOnce.Do(func() {
		level := parseLevel(logLevel)

		// Основной handler — JSON-вывод в stdout (всегда включён)
		stdoutHandler := newStdoutHandler(level)

		// Итоговый handler: либо только stdout, либо fanout (stdout + OTLP)
		handler := stdoutHandler

		// Если OTLP включён и доступен — Fanout дублирует каждую запись в оба handler'а
		if enableOTLP {
			otelHandler := newOTLPExportHandler()
			// Graceful degradation: если OTLP недоступен, otelHandler == nil — остаёмся на stdout
			if otelHandler != nil {
				handler = slogmulti.Fanout(stdoutHandler, otelHandler)
			}
		}

		slog.SetDefault(slog.New(handler))
	})
}

// Close завершает работу OTLP provider, отправляя оставшиеся логи
func Close() error {
	if otelProvider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return otelProvider.Shutdown(ctx)
}

// newStdoutHandler — JSON-логгер в stdout
// AddSource: true добавляет в каждый лог файл и строку вызова
func newStdoutHandler(level slog.Level) slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level, // минимальный уровень логирования (DEBUG, INFO, WARN, ERROR)
		AddSource: true,  // включает поле "source" с файлом/строкой вызова
	})
}

// newOTLPExportHandler создаёт OTLP handler с gRPC экспортером
// При ошибке возвращает nil
func newOTLPExportHandler() slog.Handler {
	ctx := context.Background()

	// gRPC-экспортер — отправляет логи в OTLP коллектор (Alloy, OTEL Collector и т.д.)
	exporter, err := otlploggrpc.New(
		ctx,
		otlploggrpc.WithEndpoint(otlpEndpoint()), // адрес коллектора (host:port)
		otlploggrpc.WithInsecure(),               // без TLS (для локальной разработки)
	)
	if err != nil {
		// slog ещё не инициализирован, пишем в stderr напрямую
		fmt.Fprintf(os.Stderr, "logger: не удалось создать OTLP экспортер: %v\n", err)
		return nil
	}

	// Resource — метаданные сервиса, которые прикрепляются к каждому логу
	rs, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),                               // service.name — по нему ищем логи в Grafana/Loki
			attribute.String("deployment.environment", serviceEnvironment), // окружение (dev/staging/prod)
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: не удалось создать OTel ресурс: %v\n", err)
		return nil
	}

	// LoggerProvider — управляет жизненным циклом экспортера и батчингом логов
	//
	// BatchProcessor копит записи в очереди и отправляет пачками, а не по одной
	// Дефолтные параметры (из OTel SDK):
	//   - MaxExportBatchSize = 512   — максимум записей в одной пачке
	//   - ExportInterval     = 1s    — как часто сбрасывать накопленные записи
	//   - ExportTimeout      = 30s   — таймаут на одну отправку пачки
	//   - MaxQueueSize       = 2048  — размер внутренней очереди (при переполнении записи теряются)
	//
	// Переопределить можно через опции, например:
	//   otelLogSdk.NewBatchProcessor(exporter,
	//       otelLogSdk.WithExportInterval(5 * time.Second),
	//       otelLogSdk.WithExportMaxBatchSize(1024),
	//   )
	provider := otelLogSdk.NewLoggerProvider(
		otelLogSdk.WithResource(rs),                                      // привязываем метаданные сервиса
		otelLogSdk.WithProcessor(otelLogSdk.NewBatchProcessor(exporter)), // батчинг с дефолтными параметрами (см. выше)
	)
	otelProvider = provider // сохраняем для Shutdown при завершении приложения

	// otelslog.NewHandler — официальный бридж из OpenTelemetry, который конвертирует
	// slog-записи в OTel Log Records (маппинг severity, атрибутов и т.д.)
	return otelslog.NewHandler(
		"app",
		otelslog.WithLoggerProvider(provider),
	)
}

// otlpEndpoint возвращает адрес OTLP коллектора
// Сначала проверяет стандартную переменную окружения, иначе — дефолт
func otlpEndpoint() string {
	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
		return ep
	}

	return defaultOTLPEndpoint
}

// parseLevel парсит строковый уровень ("debug", "info", "warn", "error") в slog.Level
// При невалидном значении возвращает INFO как безопасный дефолт
func parseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}

	return level
}
