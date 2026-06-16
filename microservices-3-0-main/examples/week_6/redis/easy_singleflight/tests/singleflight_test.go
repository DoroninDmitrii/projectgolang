package tests

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/easy_singleflight/internal/cache"
)

var productCache *cache.Cache

const defaultCacheTTL = 30 * time.Second

// TestMain поднимает Redis через testcontainers
func TestMain(m *testing.M) {
	os.Exit(setupAndRun(m))
}

func setupAndRun(m *testing.M) int {
	_ = godotenv.Load("../.env")

	ctx := context.Background()

	redisContainer, err := tcredis.Run(ctx, os.Getenv("REDIS_IMAGE"))
	if err != nil {
		slog.Error("не удалось поднять Redis контейнер", "error", err)
		return 1
	}
	defer func() { _ = redisContainer.Terminate(ctx) }()

	redisAddr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		slog.Error("не удалось получить адрес Redis", "error", err)
		return 1
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	if pingErr := rdb.Ping(ctx).Err(); pingErr != nil {
		slog.Error("не удалось подключиться к Redis", "error", pingErr)
		return 1
	}

	slog.Info("Redis контейнер запущен", "addr", redisAddr)

	productCache = cache.New(rdb, defaultCacheTTL)

	return m.Run()
}

// runConcurrent запускает N горутин, каждая выполняет fn
func runConcurrent(n int, fn func()) {
	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	wg.Wait()
}

// newFetchFromDB создаёт тестовый fetchFromDB с счётчиком вызовов и возможностью инъекции ошибок
func newFetchFromDB(dbCallCount *atomic.Int64, errToReturn error) cache.FetchFunc {
	return func(_ context.Context, productID string) (*cache.Product, error) {
		dbCallCount.Add(1)

		// Симуляция задержки БД (time.Sleep запрещён линтером — используем таймер)
		timer := time.NewTimer(100 * time.Millisecond)
		<-timer.C

		if errToReturn != nil {
			return nil, errToReturn
		}

		return &cache.Product{
			ID:    productID,
			Name:  "Товар " + productID,
			Price: 9990,
		}, nil
	}
}

// TestStampede_WithoutSingleflight проверяет, что без singleflight
// каждая горутина обращается к БД (cache stampede)
func TestStampede_WithoutSingleflight(t *testing.T) {
	ctx := context.Background()
	productID := "stampede-no-sf"

	_ = productCache.Del(ctx, productID)

	var dbCalls atomic.Int64

	fetchFn := newFetchFromDB(&dbCalls, nil)

	const numGoroutines = 20

	runConcurrent(numGoroutines, func() {
		_, err := productCache.GetProduct(ctx, productID, fetchFn)
		assert.NoError(t, err)
	})

	// Без singleflight большинство горутин обратится к БД
	calls := dbCalls.Load()
	t.Logf("обращений к БД без singleflight: %d", calls)
	assert.GreaterOrEqual(t, calls, int64(10), "ожидаем множественные обращения к БД (stampede)")
}

// TestStampede_WithSingleflight проверяет, что с singleflight
// только 1-2 горутины обращаются к БД
func TestStampede_WithSingleflight(t *testing.T) {
	ctx := context.Background()
	productID := "stampede-with-sf"

	_ = productCache.Del(ctx, productID)

	var dbCalls atomic.Int64

	fetchFn := newFetchFromDB(&dbCalls, nil)

	const numGoroutines = 20

	runConcurrent(numGoroutines, func() {
		_, err := productCache.GetProductSF(ctx, productID, fetchFn)
		assert.NoError(t, err)
	})

	// С singleflight только 1-2 обращения к БД
	calls := dbCalls.Load()
	t.Logf("обращений к БД с singleflight: %d", calls)
	assert.LessOrEqual(t, calls, int64(2), "ожидаем 1-2 обращения к БД (singleflight)")
}

// TestCache_HitAfterWarmup проверяет, что прогретый кеш обслуживает запросы без обращения к БД
func TestCache_HitAfterWarmup(t *testing.T) {
	ctx := context.Background()
	productID := "cache-warmup"

	// Прогреваем кеш
	product := &cache.Product{ID: productID, Name: "Прогретый товар", Price: 5500}
	err := productCache.Set(ctx, product)
	require.NoError(t, err)

	var dbCalls atomic.Int64

	fetchFn := newFetchFromDB(&dbCalls, nil)

	const numGoroutines = 20

	runConcurrent(numGoroutines, func() {
		_, err := productCache.GetProduct(ctx, productID, fetchFn)
		assert.NoError(t, err)
	})

	// С прогретым кешем — 0 обращений к БД
	assert.Equal(t, int64(0), dbCalls.Load(), "прогретый кеш не должен обращаться к БД")
}

// TestSingleflight_ErrorPropagation проверяет, что ошибка БД
// распространяется на все ожидающие горутины
func TestSingleflight_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	productID := "error-propagation"

	_ = productCache.Del(ctx, productID)

	errDB := errors.New("database connection refused")

	fetchFn := newFetchFromDB(new(atomic.Int64), errDB)

	const numGoroutines = 10

	errs := make([]error, numGoroutines)

	var wg sync.WaitGroup

	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func() {
			defer wg.Done()

			_, errs[i] = productCache.GetProductSF(ctx, productID, fetchFn)
		}()
	}

	wg.Wait()

	// Все горутины должны получить ошибку
	for i, err := range errs {
		assert.ErrorIs(t, err, errDB, "горутина %d должна получить ошибку БД", i)
	}
}

// TestCache_SetAndGet проверяет базовый round-trip кеша
func TestCache_SetAndGet(t *testing.T) {
	ctx := context.Background()

	product := &cache.Product{
		ID:    "roundtrip-test",
		Name:  "Тестовый товар",
		Price: 1234,
	}

	// Записываем в кеш
	err := productCache.Set(ctx, product)
	require.NoError(t, err)

	// Читаем из кеша
	cached, err := productCache.Get(ctx, product.ID)
	require.NoError(t, err)
	require.NotNil(t, cached)

	assert.Equal(t, product.ID, cached.ID)
	assert.Equal(t, product.Name, cached.Name)
	assert.Equal(t, product.Price, cached.Price)
}
