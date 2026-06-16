package redis_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	platformRedis "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/platform_redis/platform/pkg/redis"
)

var redisAddr string

func TestMain(m *testing.M) {
	_ = godotenv.Load("../../../.env")

	ctx := context.Background()

	container, err := redis.Run(ctx, os.Getenv("REDIS_IMAGE"))
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

	redisAddr = host + ":" + port.Port()

	code := m.Run()

	_ = container.Terminate(ctx)

	os.Exit(code)
}

func TestNewClient_Success(t *testing.T) {
	rdb, err := platformRedis.NewClient(&goredis.Options{Addr: redisAddr})
	require.NoError(t, err)
	defer rdb.Close()

	// Проверяем, что клиент работает
	err = rdb.Set(context.Background(), "test:key", "value", time.Minute).Err()
	require.NoError(t, err)

	val, err := rdb.Get(context.Background(), "test:key").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestNewClient_BadAddress(t *testing.T) {
	_, err := platformRedis.NewClient(&goredis.Options{
		Addr:        "localhost:1",
		DialTimeout: 500 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось подключиться к Redis")
}

func TestNewClient_DefaultTimeouts(t *testing.T) {
	opts := &goredis.Options{Addr: redisAddr}

	rdb, err := platformRedis.NewClient(opts)
	require.NoError(t, err)
	defer rdb.Close()

	// После NewClient дефолтные таймауты должны быть выставлены
	assert.Equal(t, 5*time.Second, opts.DialTimeout)
	assert.Equal(t, 3*time.Second, opts.ReadTimeout)
	assert.Equal(t, 3*time.Second, opts.WriteTimeout)
	assert.True(t, opts.ContextTimeoutEnabled)
}

func TestNewClient_CustomTimeouts(t *testing.T) {
	opts := &goredis.Options{
		Addr:         redisAddr,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  7 * time.Second,
		WriteTimeout: 7 * time.Second,
	}

	rdb, err := platformRedis.NewClient(opts)
	require.NoError(t, err)
	defer rdb.Close()

	// Пользовательские таймауты НЕ должны быть перезаписаны
	assert.Equal(t, 10*time.Second, opts.DialTimeout)
	assert.Equal(t, 7*time.Second, opts.ReadTimeout)
	assert.Equal(t, 7*time.Second, opts.WriteTimeout)
}

func TestNewClient_WithAllObservability(t *testing.T) {
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

	// SET — должен создать спан и записать метрику
	err = rdb.Set(ctx, "obs:key", "value", time.Minute).Err()
	require.NoError(t, err)

	// GET — должен создать спан и записать метрику
	val, err := rdb.Get(ctx, "obs:key").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// GET несуществующего ключа — redis.Nil НЕ должен записываться как ошибка
	_, err = rdb.Get(ctx, "obs:nonexistent").Result()
	assert.ErrorIs(t, err, goredis.Nil)

	// DEL
	err = rdb.Del(ctx, "obs:key").Err()
	require.NoError(t, err)
}

func TestNewClient_WithTracingOnly(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background()) //nolint:errcheck // Shutdown в тесте

	rdb, err := platformRedis.NewClient(
		&goredis.Options{Addr: redisAddr},
		platformRedis.WithTracing(),
	)
	require.NoError(t, err)
	defer rdb.Close()

	err = rdb.Set(context.Background(), "trace:key", "val", time.Minute).Err()
	require.NoError(t, err)

	rdb.Del(context.Background(), "trace:key")
}

func TestNewClient_WithMetricsOnly(t *testing.T) {
	mp := sdkmetric.NewMeterProvider()
	otel.SetMeterProvider(mp)
	defer mp.Shutdown(context.Background()) //nolint:errcheck // Shutdown в тесте

	rdb, err := platformRedis.NewClient(
		&goredis.Options{Addr: redisAddr},
		platformRedis.WithMetrics(),
	)
	require.NoError(t, err)
	defer rdb.Close()

	err = rdb.Set(context.Background(), "metric:key", "val", time.Minute).Err()
	require.NoError(t, err)

	rdb.Del(context.Background(), "metric:key")
}

func TestNewClient_WithLoggingOnly(t *testing.T) {
	rdb, err := platformRedis.NewClient(
		&goredis.Options{Addr: redisAddr},
		platformRedis.WithLogging(100*time.Millisecond),
	)
	require.NoError(t, err)
	defer rdb.Close()

	err = rdb.Set(context.Background(), "log:key", "val", time.Minute).Err()
	require.NoError(t, err)

	rdb.Del(context.Background(), "log:key")
}

func TestNewClient_Pipeline(t *testing.T) {
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
	)
	require.NoError(t, err)
	defer rdb.Close()

	ctx := context.Background()

	// Pipeline — несколько команд за одно обращение
	pipe := rdb.Pipeline()
	pipe.Set(ctx, "pipe:a", "alpha", time.Minute)
	pipe.Set(ctx, "pipe:b", "bravo", time.Minute)
	getCmd := pipe.Get(ctx, "pipe:a")

	_, err = pipe.Exec(ctx)
	require.NoError(t, err)

	assert.Equal(t, "alpha", getCmd.Val())

	// Cleanup
	rdb.Del(ctx, "pipe:a", "pipe:b")
}
