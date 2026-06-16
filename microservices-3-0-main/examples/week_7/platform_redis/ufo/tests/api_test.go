package tests

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	platformRedis "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/platform_redis/platform/pkg/redis"
)

var redisAddr string

func TestMain(m *testing.M) {
	_ = godotenv.Load("../../../.env")

	ctx := context.Background()

	container, err := tcredis.Run(ctx, os.Getenv("REDIS_IMAGE"))
	if err != nil {
		slog.Error("не удалось запустить Redis контейнер", "error", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		slog.Error("не удалось получить host контейнера", "error", err)
		os.Exit(1)
	}

	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		slog.Error("не удалось получить порт контейнера", "error", err)
		os.Exit(1)
	}

	redisAddr = fmt.Sprintf("%s:%s", host, port.Port())

	code := m.Run()

	_ = container.Terminate(ctx)

	os.Exit(code)
}

// TestSetGet проверяет базовые операции SET и GET через platform Redis-клиент
func TestSetGet(t *testing.T) {
	rdb := newClient(t)
	defer rdb.Close()

	ctx := context.Background()

	err := rdb.Set(ctx, "ufo:sighting:1", `{"location":"Roswell","year":1947}`, time.Hour).Err()
	require.NoError(t, err)

	val, err := rdb.Get(ctx, "ufo:sighting:1").Result()
	require.NoError(t, err)
	assert.Equal(t, `{"location":"Roswell","year":1947}`, val)
}

// TestGetNonExistent проверяет, что GET несуществующего ключа возвращает redis.Nil
func TestGetNonExistent(t *testing.T) {
	rdb := newClient(t)
	defer rdb.Close()

	_, err := rdb.Get(context.Background(), "ufo:sighting:nonexistent").Result()
	assert.ErrorIs(t, err, goredis.Nil)
}

// TestDel проверяет операцию DEL
func TestDel(t *testing.T) {
	rdb := newClient(t)
	defer rdb.Close()

	ctx := context.Background()

	err := rdb.Set(ctx, "ufo:del:key", "value", time.Hour).Err()
	require.NoError(t, err)

	err = rdb.Del(ctx, "ufo:del:key").Err()
	require.NoError(t, err)

	_, err = rdb.Get(ctx, "ufo:del:key").Result()
	assert.ErrorIs(t, err, goredis.Nil)
}

// TestPipeline проверяет Pipeline — несколько команд за одно обращение к серверу
func TestPipeline(t *testing.T) {
	rdb := newClient(t)
	defer rdb.Close()

	ctx := context.Background()

	pipe := rdb.Pipeline()
	pipe.Set(ctx, "ufo:pipe:a", "alpha", time.Minute)
	pipe.Set(ctx, "ufo:pipe:b", "bravo", time.Minute)
	getCmd := pipe.Get(ctx, "ufo:pipe:a")

	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	assert.Equal(t, "alpha", getCmd.Val())

	// Проверяем, что оба ключа записаны
	val, err := rdb.Get(ctx, "ufo:pipe:b").Result()
	require.NoError(t, err)
	assert.Equal(t, "bravo", val)

	// Cleanup
	rdb.Del(ctx, "ufo:pipe:a", "ufo:pipe:b")
}

// TestWithObservability проверяет, что клиент с полным observability
// (трейсы, метрики, логи) корректно выполняет Redis-операции
func TestWithObservability(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background()) //nolint:errcheck // Shutdown в тесте

	mp := sdkmetric.NewMeterProvider()
	otel.SetMeterProvider(mp)
	defer mp.Shutdown(context.Background()) //nolint:errcheck // Shutdown в тесте

	rdb, err := platformRedis.NewClient(
		&goredis.Options{Addr: redisAddr},
		platformRedis.WithTracing(),
		platformRedis.WithMetrics(),
		platformRedis.WithLogging(100*time.Millisecond),
	)
	require.NoError(t, err)
	defer rdb.Close()

	ctx := context.Background()

	// SET
	err = rdb.Set(ctx, "obs:key", `{"location":"Area51"}`, time.Minute).Err()
	require.NoError(t, err)

	// GET
	val, err := rdb.Get(ctx, "obs:key").Result()
	require.NoError(t, err)
	assert.Equal(t, `{"location":"Area51"}`, val)

	// GET несуществующего ключа — redis.Nil НЕ должен записываться как ошибка
	_, err = rdb.Get(ctx, "obs:nonexistent").Result()
	assert.ErrorIs(t, err, goredis.Nil)

	// DEL
	err = rdb.Del(ctx, "obs:key").Err()
	require.NoError(t, err)

	// Pipeline
	pipe := rdb.Pipeline()
	pipe.Set(ctx, "obs:a", "alpha", time.Minute)
	pipe.Set(ctx, "obs:b", "bravo", time.Minute)
	_, err = pipe.Exec(ctx)
	require.NoError(t, err)

	// Cleanup
	rdb.Del(ctx, "obs:a", "obs:b")
}

// TestTTL проверяет, что TTL корректно выставляется через SET
func TestTTL(t *testing.T) {
	rdb := newClient(t)
	defer rdb.Close()

	ctx := context.Background()

	err := rdb.Set(ctx, "ufo:ttl:key", "value", 10*time.Second).Err()
	require.NoError(t, err)

	ttl, err := rdb.TTL(ctx, "ufo:ttl:key").Result()
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 10*time.Second, "TTL должен быть в диапазоне (0, 10s], получили: %v", ttl)

	// Cleanup
	rdb.Del(ctx, "ufo:ttl:key")
}

// newClient создаёт platform Redis-клиент без observability для базовых тестов
func newClient(t *testing.T) *goredis.Client {
	t.Helper()

	rdb, err := platformRedis.NewClient(&goredis.Options{Addr: redisAddr})
	require.NoError(t, err)

	return rdb
}
