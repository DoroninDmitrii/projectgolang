# Distributed Rate Limiter -- глобальное ограничение через Redis

Пример распределённого rate limiting через `go-redis/redis_rate/v10`. Три реплики сервиса за Nginx разделяют общий счётчик в Redis. Лимиты задаются per-method: записи ограничены строже, чем чтения.

## Концепция

Локальный rate limiter (см. `easy_rate_limiter`) работает per-instance: при 3 репликах с лимитом 10 RPS реальный лимит -- 30 RPS. Распределённый rate limiter решает эту проблему, храня счётчик в Redis.

**GCRA (Generic Cell Rate Algorithm)** -- вариант token bucket, реализованный как атомарный Lua-скрипт в Redis. Один вызов `limiter.Allow()` = один запрос в Redis, который атомарно проверяет и обновляет счётчик. Никаких race conditions между репликами.

**Per-method лимиты.** Записи дороже чтений -- разумно ограничивать их строже:

| Метод | Лимит | Причина |
|-------|-------|---------|
| `CreateUfo` | 5 RPS | Мутирующая операция |
| `GetUfo` | 50 RPS | Read-only, масштабируется |
| `ListUfo` | 10 RPS | Default для незарегистрированных методов |

**Fail-open.** Если Redis недоступен -- запрос пропускается. Лучше обслужить без лимита, чем отказать всем.

## Архитектура

```
ghz (load test)
    │
    ▼
  Nginx (:50051, HTTP/2 round-robin)
    │
    ├──► app:1 ──┐
    ├──► app:2 ──┤──► Redis (GCRA Lua script)
    └──► app:3 ──┘
           │
           ▼
     OTel Collector → Prometheus → Grafana (:3000)
```

Nginx использует Docker DNS: `server app:50051` резолвится во все реплики автоматически. Масштабирование: `--scale app=N`.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Distributed interceptor | `internal/ratelimit/grpc_interceptor.go` | Per-method lookup + `redis_rate.Limiter.Allow()` + fail-open при ошибке Redis |
| Per-method конфигурация | `cmd/grpc_server/main.go:66-73` | Map `FullMethod → redis_rate.Limit` с разными лимитами |
| Nginx gRPC LB | `nginx.conf` | `grpc_pass grpc://grpc_backend` + Docker DNS resolver |
| Multi-stage Dockerfile | `Dockerfile` | golang:1.26-alpine (build) → alpine:3.21 (run), ~10 MB |
| Тесты с testcontainers | `tests/api_test.go` | Реальный Redis в контейнере, очистка ключей `rate:*` между тестами |
| Параллельный load test | `Taskfile.yaml:173-187` | Три ghz-процесса одновременно -- видны per-method лимиты в Grafana |
| ServiceInstanceID | `cmd/grpc_server/main.go` | Hostname контейнера в метриках -- различаем реплики в Grafana |

## Как запустить

```bash
task up             # Nginx + 3 реплики + Redis + Prometheus + Grafana + OTel Collector
task load:grpc      # нагрузочный тест всех 3 RPC параллельно
task test:api       # тесты (поднимают Redis через testcontainers)
task down
```

Grafana: `localhost:3000` (admin/admin). В графиках видно, как CreateUfo начинает отбрасываться при ~5 RPS, а GetUfo держит ~50 RPS.

## Структура проекта

```
distributed_rate_limiter/
├── cmd/grpc_server/        # сервер с redis_rate + per-method лимиты
├── internal/
│   ├── ratelimit/          # distributed interceptor (GCRA через Redis)
│   ├── api/                # обработчики (CreateUfo, GetUfo, ListUfo)
│   └── metrics/            # OTel MeterProvider с ServiceInstanceID
├── nginx.conf              # HTTP/2 gRPC load balancer
├── Dockerfile              # multi-stage build
├── docker-compose.yml      # полный стек: Nginx, 3x app, Redis, Prometheus, Grafana, OTel
└── tests/                  # testcontainers + реальный Redis
```
