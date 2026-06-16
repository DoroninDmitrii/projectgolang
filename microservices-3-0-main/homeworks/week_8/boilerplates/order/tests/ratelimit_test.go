//go:build apitest

// Package tests содержит интеграционный API-тест distributed rate limiter
//
// Под одним рабочим Redis-контейнером (testcontainers) проверяем,
// что при превышении лимита middleware возвращает 429, и что при
// недоступности Redis работает fail-open (запрос проходит дальше)
package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/student/platform/pkg/ratelimit"
)

// startRateLimitRedis поднимает Redis в testcontainer и возвращает host:port.
// Контейнер останавливается через t.Cleanup
func startRateLimitRedis(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:8.6.1-alpine3.23")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(container)
	})

	addr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	// ConnectionString отдаёт redis://host:port — обрезаем схему
	const prefix = "redis://"
	addr = strings.TrimPrefix(addr, prefix)

	return addr
}

// okHandler — простой stub-обработчик: всегда возвращает 200
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRateLimit_429AfterBurstExceeded(t *testing.T) {
	addr := startRateLimitRedis(t)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := redis_rate.NewLimiter(rdb)

	// Маленький лимит — чтобы стабильно поймать 429 за десяток запросов.
	// rate=2 означает 2 запроса в секунду; burst=2 — максимум 2 запроса всплеском
	const rate = 2
	const burst = 2

	ts := httptest.NewServer(ratelimit.Middleware(limiter, rate, burst)(okHandler()))
	t.Cleanup(ts.Close)

	// Делаем 10 запросов на один путь подряд — первые burst пройдут,
	// остальные должны получить 429
	const totalRequests = 10
	statuses := make([]int, 0, totalRequests)
	for range totalRequests {
		resp, err := http.Get(ts.URL + "/limited")
		require.NoError(t, err)
		statuses = append(statuses, resp.StatusCode)
		_ = resp.Body.Close()
	}

	var ok, tooMany int
	for _, s := range statuses {
		switch s {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			tooMany++
		}
	}
	require.Greater(t, ok, 0, "хотя бы один запрос должен был пройти: %v", statuses)
	require.Greater(t, tooMany, 0, "хотя бы один запрос должен был получить 429: %v", statuses)
	require.LessOrEqual(t, ok, burst+1,
		"нельзя пропускать больше burst (+1 на toleration GCRA refill): %v", statuses)
}

func TestRateLimit_FailOpen_OnRedisUnavailable(t *testing.T) {
	// 127.0.0.1:1 — привилегированный порт, никем не слушается → ECONNREFUSED.
	// Это проще, чем поднимать testcontainer и сразу гасить его, и стабильнее
	// (порт 0 был бы перенаправлен на динамический; порт 1 точно мёртвый).
	rdb := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := redis_rate.NewLimiter(rdb)

	ts := httptest.NewServer(ratelimit.Middleware(limiter, 100, 200)(okHandler()))
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/whatever")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Fail-open: rate limiter не смог достучаться до Redis,
	// запрос всё равно должен пройти до handler'а
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestRateLimit_Distributed_LimitSharedBetweenInstances — центральный сценарий
// недели 8 по hw.md: «убедиться, что лимит общий (не x3 при 3 инстансах)».
//
// Поднимаем один Redis, создаём 3 независимых redis_rate.Limiter / 3 httptest
// сервера — все они работают через тот же Redis-ключ. Суммарно через них должно
// пройти ровно `burst` запросов, а не `burst × 3` — это и есть distributed-эффект.
func TestRateLimit_Distributed_LimitSharedBetweenInstances(t *testing.T) {
	addr := startRateLimitRedis(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	const (
		instances = 3
		rate      = 5 // 5 rps
		burst     = 5
	)

	servers := make([]*httptest.Server, instances)
	for i := range instances {
		// КАЖДЫЙ инстанс получает СВОЙ redis_rate.NewLimiter — но они работают через
		// общий Redis-клиент и общий ключ (path), поэтому лимит расходуется суммарно.
		limiter := redis_rate.NewLimiter(rdb)
		servers[i] = httptest.NewServer(ratelimit.Middleware(limiter, rate, burst)(okHandler()))
	}
	t.Cleanup(func() {
		for _, s := range servers {
			s.Close()
		}
	})

	const totalRequests = 30 // намного больше burst*instances
	const path = "/distributed"

	var (
		ok      atomic.Int64
		tooMany atomic.Int64
		others  atomic.Int64
	)

	// Идём последовательно по инстансам: i-й запрос летит в i%3-й сервер.
	// Цель — увидеть, что суммарно через все три проходит не больше burst+tolerance.
	for i := range totalRequests {
		srv := servers[i%instances]
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		switch resp.StatusCode {
		case http.StatusOK:
			ok.Add(1)
		case http.StatusTooManyRequests:
			tooMany.Add(1)
		default:
			others.Add(1)
		}
		_ = resp.Body.Close()
	}

	assert.Zero(t, others.Load(),
		"ожидались только 200/429, прочих кодов быть не должно")
	assert.Positive(t, ok.Load(), "хотя бы один запрос должен пройти")
	assert.Positive(t, tooMany.Load(), "хотя бы один запрос должен получить 429")

	// Главная проверка distributed-семантики: суммарно через 3 инстанса прошло
	// не больше burst+1, а не burst*3. Если бы каждый инстанс держал свой счётчик,
	// мы бы увидели до 15 успешных ответов.
	assert.LessOrEqualf(t, ok.Load(), int64(burst+1),
		"distributed: суммарно через %d инстансов должно пройти не больше burst+tolerance, "+
			"а не burst×instances=%d. Получено: ok=%d tooMany=%d",
		instances, burst*instances, ok.Load(), tooMany.Load())
}

// TestRateLimit_PerPath_IndependentLimits: middleware ключует лимит по
// r.URL.Path. Запросы на /foo не должны мешать запросам на /bar даже если
// /foo уже исчерпал burst.
func TestRateLimit_PerPath_IndependentLimits(t *testing.T) {
	addr := startRateLimitRedis(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := redis_rate.NewLimiter(rdb)

	const rate = 1
	const burst = 1

	ts := httptest.NewServer(ratelimit.Middleware(limiter, rate, burst)(okHandler()))
	t.Cleanup(ts.Close)

	// /foo: bомбим до 429.
	for range 5 {
		resp, err := http.Get(ts.URL + "/foo")
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	// /bar должен получить 200 — у него собственный счётчик.
	respBar, err := http.Get(ts.URL + "/bar")
	require.NoError(t, err)
	defer respBar.Body.Close()

	assert.Equal(t, http.StatusOK, respBar.StatusCode,
		"per-path rate limit: /bar должен иметь независимый счётчик от /foo")
}

// TestRateLimit_Concurrency_AtomicGCRA: 50 параллельных горутин на один path.
// Атомарность GCRA в redis_rate должна гарантировать, что суммарно проходит
// не больше burst+tolerance — независимо от порядка планировщика.
func TestRateLimit_Concurrency_AtomicGCRA(t *testing.T) {
	addr := startRateLimitRedis(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := redis_rate.NewLimiter(rdb)

	const rate = 3
	const burst = 3

	ts := httptest.NewServer(ratelimit.Middleware(limiter, rate, burst)(okHandler()))
	t.Cleanup(ts.Close)

	const goroutines = 50
	var (
		wg      sync.WaitGroup
		ok      atomic.Int64
		tooMany atomic.Int64
		others  atomic.Int64
	)

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/concurrent")
			if err != nil {
				others.Add(1)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			switch resp.StatusCode {
			case http.StatusOK:
				ok.Add(1)
			case http.StatusTooManyRequests:
				tooMany.Add(1)
			default:
				others.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Zero(t, others.Load(), "ожидались только 200/429")
	assert.Positive(t, ok.Load(), "хотя бы один запрос должен пройти")
	assert.Positive(t, tooMany.Load(), "большая часть должна быть отброшена 429")

	// burst+1 — допуск на GCRA refill за время выполнения 50 параллельных запросов
	// (несколько миллисекунд). Главное — мы не получили 50 успешных ответов из 50.
	assert.LessOrEqualf(t, ok.Load(), int64(burst+1),
		"concurrency: GCRA должен оставаться атомарным под нагрузкой. ok=%d tooMany=%d",
		ok.Load(), tooMany.Load())
}
