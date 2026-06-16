// Демо-приложение, показывающее использование Platform Redis с observability
//
// Что делает:
//  1. Инициализирует OpenTelemetry (трейсы → Jaeger, метрики → Prometheus, логи → Elasticsearch через OTel Collector)
//  2. Создаёт Redis-клиент через платформенную библиотеку с тремя аспектами observability
//  3. Выполняет несколько Redis-команд — трейсы, метрики и логи появляются автоматически
//
// Запуск:
//
//	# Поднять инфраструктуру (Redis + Jaeger + Prometheus + Grafana + Elasticsearch + Kibana + OTel Collector)
//	task up
//
//	# Запустить демо
//	go run ./cmd/main.go
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	platformLogger "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/platform_redis/platform/pkg/logger"
	platformMetrics "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/platform_redis/platform/pkg/metrics"
	platformRedis "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/platform_redis/platform/pkg/redis"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/platform_redis/platform/pkg/tracing"
)

const (
	serviceName    = "ufo-service"
	serviceVersion = "1.0.0"
	environment    = "development"
)

func initObservability(ctx context.Context) (func(), error) {
	collectorEndpoint := cmp.Or(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "localhost:4317")

	// Инициализируем логгер через платформенную библиотеку
	// Логи пишутся в stdout (JSON) + OTLP → OTel Collector → Elasticsearch (Kibana: http://localhost:5601)
	platformLogger.Init(platformLogger.Config{
		Level:             "info",
		ServiceName:       serviceName,
		Environment:       environment,
		EnableOTLP:        true,
		CollectorEndpoint: collectorEndpoint,
	})

	// Инициализируем трейсинг через платформенную библиотеку
	// Трейсы отправляются в OTel Collector → Jaeger (UI: http://localhost:16686)
	shutdownTracer, err := tracing.InitTracer(ctx, tracing.Config{
		CollectorEndpoint: collectorEndpoint,
		ServiceName:       serviceName,
		Environment:       environment,
		ServiceVersion:    serviceVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("не удалось инициализировать трейсер: %w", err)
	}

	// Инициализируем OTel Metrics через платформенную библиотеку
	// Метрики отправляются в OTel Collector → Prometheus (UI: http://localhost:9091)
	// MeterProvider с OTLP-экспортером регистрируется глобально —
	// все Meter'ы (включая platform/redis) автоматически его используют
	platformMetrics.Init(
		serviceName,
		platformMetrics.WithCollectorEndpoint(collectorEndpoint),
	)

	cleanup := func() {
		if logErr := platformLogger.Close(); logErr != nil {
			slog.Error("не удалось завершить логгер", "error", logErr)
		}
		if tracerErr := shutdownTracer(ctx); tracerErr != nil {
			slog.Error("не удалось завершить трейсер", "error", tracerErr)
		}
		if metricsErr := platformMetrics.Close(); metricsErr != nil {
			slog.Error("не удалось завершить MeterProvider", "error", metricsErr)
		}
	}

	return cleanup, nil
}

func initRedisClient() (*redis.Client, error) {
	// Создаём Redis-клиент через платформенную библиотеку
	// WithTracing()  — спаны на каждую команду
	// WithMetrics()  — histogram длительности + counter ошибок
	// WithLogging()  — логирование медленных (>50ms) и ошибочных команд
	rdb, err := platformRedis.NewClient(
		&redis.Options{
			Addr: cmp.Or(os.Getenv("REDIS_ADDR"), "localhost:6379"),
		},
		platformRedis.WithTracing(),
		platformRedis.WithMetrics(),
		platformRedis.WithLogging(50*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать Redis-клиент: %w", err)
	}

	return rdb, nil
}

// runDemo генерирует непрерывный поток Redis-команд, имитируя реальную нагрузку
// Работает до отмены контекста (Ctrl+C). Каждую итерацию выполняет SET, GET (hit),
// GET несуществующего ключа (miss), DEL и Pipeline — чтобы на дашборде были видны
// все типы метрик: hit rate, command duration, throughput
func runDemo(ctx context.Context, rdb *redis.Client) {
	slog.Info("=== Демонстрация Platform Redis с Observability ===")
	slog.Info("генерируем нагрузку, нажмите Ctrl+C для остановки")
	slog.Info("трейсы: http://localhost:16686 | метрики: http://localhost:9091 | дашборд: http://localhost:3000")

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	var iteration int
	for {
		select {
		case <-ctx.Done():
			slog.Info("остановка генерации нагрузки", "iterations", iteration)
			return
		case <-ticker.C:
			iteration++
			runIteration(ctx, rdb, iteration)
		}
	}
}

func runIteration(ctx context.Context, rdb *redis.Client, iteration int) {
	key := fmt.Sprintf("ufo:sighting:%d", iteration%10)

	// SET — создаём запись в кэше
	if err := rdb.Set(ctx, key, fmt.Sprintf(`{"location":"Zone-%d","year":2024}`, iteration), time.Minute).Err(); err != nil {
		slog.Error("ошибка SET", "error", err)
		return
	}

	// GET — читаем запись из кэша (hit)
	if _, err := rdb.Get(ctx, key).Result(); err != nil {
		slog.Error("ошибка GET", "error", err)
	}

	// GET несуществующего ключа (miss) — с вероятностью 30%.
	// Имитируем реалистичный hit rate ~70-80%.
	n, err := rand.Int(rand.Reader, big.NewInt(100))
	if err != nil {
		slog.Error("ошибка генерации случайного числа", "error", err)
		return
	}

	if n.Int64() < 30 {
		missKey, randErr := rand.Int(rand.Reader, big.NewInt(1000))
		if randErr != nil {
			slog.Error("ошибка генерации случайного числа", "error", randErr)
			return
		}

		if _, getErr := rdb.Get(ctx, fmt.Sprintf("ufo:nonexistent:%d", missKey.Int64())).Result(); getErr != nil {
			// промах ожидаем, ошибка redis.Nil — нормальное поведение
			slog.Debug("cache miss", "error", getErr)
		}
	}

	// Pipeline — раз в 5 итераций
	if iteration%5 == 0 {
		pipe := rdb.Pipeline()
		pipe.Set(ctx, "ufo:a", "alpha", time.Minute)
		pipe.Set(ctx, "ufo:b", "bravo", time.Minute)
		pipe.Get(ctx, "ufo:a")
		if _, err = pipe.Exec(ctx); err != nil {
			slog.Error("ошибка Pipeline", "error", err)
		}
	}

	// DEL — раз в 10 итераций, чтобы создать будущие промахи
	if iteration%10 == 0 {
		if err = rdb.Del(ctx, key).Err(); err != nil {
			slog.Error("ошибка DEL", "error", err)
		}
	}

	if iteration%20 == 0 {
		slog.Info("прогресс", "iterations", iteration)
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cleanupObservability, err := initObservability(ctx)
	if err != nil {
		slog.Error("не удалось инициализировать observability", "error", err)
		return
	}
	defer cleanupObservability()

	rdb, err := initRedisClient()
	if err != nil {
		slog.Error("не удалось создать Redis-клиент", "error", err)
		return
	}
	defer rdb.Close()

	runDemo(ctx, rdb)
}
