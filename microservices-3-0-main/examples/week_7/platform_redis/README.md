# Platform Redis — observability-обёртка для Redis

Переиспользуемый Redis-клиент с прозрачной инструментацией: tracing, metrics, logging через hook-архитектуру go-redis. Полный observability-стек для визуализации.

## Концепция

go-redis поддерживает хуки (`redis.Hook`) — middleware, которые оборачивают каждую команду. Через них можно прозрачно добавить observability без изменения прикладного кода.

Три аспекта инструментации включаются независимо через functional options:

```go
rdb, err := platformRedis.NewClient(opts,
    platformRedis.WithTracing(),                    // OTel spans
    platformRedis.WithMetrics(),                    // histogram + counters
    platformRedis.WithLogging(50*time.Millisecond), // slow log
)
```

Важный нюанс: `redis.Nil` (ключ не найден) — это не ошибка. Hook корректно отделяет cache miss от реальных ошибок: miss считается в `redis.cache.misses`, но не попадает в `redis.command.errors` и не красит span красным.

## Архитектура

```
Go App
  └── platform Redis client (hook)
        ├── Tracing  → OTLP → Collector → Jaeger     (:16686)
        ├── Metrics  → OTLP → Collector → Prometheus  (:9090) → Grafana (:3000)
        └── Logging  → OTLP → Collector → ES          (:9200) → Kibana  (:5601)

Redis (:6379)
```

7 сервисов в docker-compose: Redis, Jaeger, Prometheus, Grafana, Elasticsearch, Kibana, OTel Collector.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Hook-архитектура | `platform/pkg/redis/hook.go` | Реализует `redis.Hook` (DialHook, ProcessHook, ProcessPipelineHook). Создаёт span, пишет метрики, логирует медленные команды |
| Functional options | `platform/pkg/redis/option.go` | `WithTracing()`, `WithMetrics()`, `WithLogging()` — composable, каждый аспект независим |
| Cache hit/miss | `platform/pkg/redis/hook.go` | `redis.Nil` отделяется от ошибок: отдельные счётчики hits/misses, span не помечается ошибкой |
| Кастомные бакеты | `platform/pkg/redis/hook.go` | Histogram бакеты оптимизированы под Redis (0.1ms–1s), не HTTP-дефолты |
| Semantic conventions | `platform/pkg/redis/hook.go` | `db.system=redis`, `db.operation.name`, `server.address` — стандартные OTel-атрибуты |
| Platform logger | `platform/pkg/logger/` | Fanout: stdout + OTLP. Graceful degradation при недоступности коллектора |
| Platform tracing | `platform/pkg/tracing/` | ParentBased sampler, W3C propagation, gzip compression, retry с backoff |
| Platform metrics | `platform/pkg/metrics/` | PeriodicReader (10s), custom Views для бакетов, глобальный MeterProvider |
| Демо-нагрузка | `ufo/cmd/main.go` | Continuous loop: SET/GET/DEL/Pipeline с 30% miss rate для реалистичных метрик |
| Тесты | `platform/pkg/redis/client_test.go` | Testcontainers: каждый аспект тестируется отдельно, проверяется корректность redis.Nil |

## Как запустить

```bash
task up              # 7 сервисов (Redis, Jaeger, Prometheus, Grafana, ES, Kibana, Collector)
task run             # демо-приложение с непрерывной нагрузкой на Redis
task test            # тесты Redis-клиента (testcontainers)
task down            # остановка
```

- Jaeger (traces): http://localhost:16686
- Grafana (metrics): http://localhost:3000
- Kibana (logs): http://localhost:5601
- Prometheus: http://localhost:9090

## Структура проекта

```
platform_redis/
├── platform/pkg/
│   ├── redis/                # клиент, hook, functional options
│   ├── tracing/              # OTel TracerProvider
│   ├── metrics/              # OTel MeterProvider
│   └── logger/               # slog + OTLP fanout
├── ufo/
│   ├── cmd/main.go           # демо-приложение с нагрузкой
│   └── tests/                # интеграционные тесты
├── deploy/docker-compose.yml # 7 сервисов
└── otel-collector-config.yaml
```
