// Package logger — платформенный компонент логирования: stdout (JSON) + OTLP → Elasticsearch
//
// Архитектура: slog с fanout-handler'ом, который дублирует каждую запись
// в два направления:
//  1. stdout — JSON-формат для локальной разработки и kubectl logs
//  2. OTLP gRPC → OTel Collector → Elasticsearch → Kibana — централизованное хранилище
//
// Компонент следует тому же паттерну, что tracing и metrics:
// Init + Close, конфигурация через Config, graceful degradation при недоступности коллектора
package logger

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otelLogSdk "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

var (
	// otelProvider хранится для корректного завершения (Close)
	otelProvider *otelLogSdk.LoggerProvider

	// initOnce гарантирует, что Init вызовется только один раз (потокобезопасность)
	initOnce sync.Once
)

// Init создаёт глобальный slog-логгер и устанавливает его через slog.SetDefault
//
// При cfg.EnableOTLP=true логи дополнительно отправляются в OTLP коллектор
// (обычно OTel Collector → Elasticsearch → Kibana)
//
// Пример использования:
//
//	logger.Init(logger.Config{
//	    Level:             "info",
//	    ServiceName:       "ufo-service",
//	    Environment:       "development",
//	    EnableOTLP:        true,
//	    CollectorEndpoint: "localhost:4317",
//	})
//	defer logger.Close()
func Init(cfg Config) {
	initOnce.Do(func() {
		level := parseLevel(cfg.Level)

		// Основной handler — JSON-вывод в stdout (всегда включён)
		// AddSource: true добавляет в каждый лог файл и строку вызова
		stdoutHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     level, // минимальный уровень логирования (DEBUG, INFO, WARN, ERROR)
			AddSource: true,  // включает поле "source" с файлом/строкой вызова
		})

		// Итоговый handler: либо только stdout, либо fanout (stdout + OTLP)
		handler := slog.Handler(stdoutHandler)

		// Если OTLP включён и доступен — Fanout дублирует каждую запись в оба handler'а
		if cfg.EnableOTLP {
			otelHandler := newOTLPExportHandler(cfg)
			// Graceful degradation: если OTLP недоступен, otelHandler == nil — остаёмся на stdout
			if otelHandler != nil {
				handler = slogmulti.Fanout(stdoutHandler, otelHandler)
			}
		}

		slog.SetDefault(slog.New(handler))
	})
}

// Close завершает работу OTLP provider, отправляя оставшиеся логи
// Вызывайте при остановке приложения (обычно через defer)
func Close() error {
	if otelProvider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()

	return otelProvider.Shutdown(ctx)
}

// newOTLPExportHandler создаёт OTLP handler с gRPC экспортером
// При ошибке возвращает nil (graceful degradation — логи продолжат идти в stdout)
func newOTLPExportHandler(cfg Config) slog.Handler {
	ctx := context.Background()

	endpoint := cmp.Or(cfg.CollectorEndpoint, defaultOTLPEndpoint)

	// gRPC-экспортер — отправляет логи в OTLP коллектор (OTel Collector и т.д.)
	exporter, err := otlploggrpc.New(
		ctx,
		otlploggrpc.WithEndpoint(endpoint), // адрес коллектора (host:port)
		otlploggrpc.WithInsecure(),         // без TLS (для локальной разработки)
	)
	if err != nil {
		// slog ещё может быть не инициализирован, пишем в stderr напрямую
		fmt.Fprintf(os.Stderr, "logger: failed to create OTLP exporter: %v\n", err)
		return nil
	}

	// Resource — метаданные сервиса, которые прикрепляются к каждому логу
	// По ним можно фильтровать в Kibana: service.name, deployment.environment
	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: failed to create OTel resource: %v\n", err)
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
	provider := otelLogSdk.NewLoggerProvider(
		otelLogSdk.WithResource(res),
		otelLogSdk.WithProcessor(otelLogSdk.NewBatchProcessor(exporter)),
	)
	otelProvider = provider // сохраняем для Shutdown при завершении приложения

	// otelslog.NewHandler — официальный бридж из OpenTelemetry, который конвертирует
	// slog-записи в OTel Log Records (маппинг severity, атрибутов и т.д.)
	return otelslog.NewHandler(
		"app",
		otelslog.WithLoggerProvider(provider),
	)
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
