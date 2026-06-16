# Rate Limiter -- ограничение частоты запросов (Token Bucket)

Пример локального rate limiting для gRPC и HTTP через `golang.org/x/time/rate`. Один лимитер на сервер, token bucket с настраиваемым RPS и burst.

## Концепция

Token Bucket -- алгоритм, совмещающий ограничение средней частоты с допуском кратковременных всплесков:

- В ведро добавляется `rate` токенов в секунду (10 RPS)
- Максимальная ёмкость ведра -- `burst` (20 токенов)
- Каждый запрос забирает 1 токен. Если ведро пустое -- отказ
- При старте ведро полное: первые 20 запросов проходят мгновенно, дальше -- 10 req/sec

`Allow()` -- неблокирующая проверка. Отклонённые запросы получают отказ мгновенно, без ожидания. Сервер остаётся отзывчивым даже под перегрузкой.

## Архитектура

```
gRPC Client ──► :50051 ──► [rate.Limiter] ──► gRPC Handler ──► OTel
HTTP Client ──► :8080  ──► [rate.Limiter] ──► Chi Handler  ──► OTel
                                                                 │
                                          OTel Collector ◄───────┘
                                               │
                                          Prometheus ◄── Remote Write
                                               │
                                            Grafana (:3000)
```

gRPC-отказ: `ResourceExhausted`. HTTP-отказ: `429 Too Many Requests`.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Интерфейс Limiter | `internal/ratelimit/limiter.go` | Один метод `Allow() bool` -- подменяем реализацию в тестах |
| gRPC interceptor | `internal/ratelimit/grpc_interceptor.go` | 5 строк: проверка + `ResourceExhausted` |
| HTTP middleware | `internal/ratelimit/http_middleware.go` | Chi middleware, 5 строк: проверка + HTTP 429 |
| HTTP-сервер с Chi | `cmd/http_server/main.go` | Отдельный сервер с теми же обработчиками через REST |
| Custom histogram buckets | `cmd/grpc_server/main.go` | Дефолтные OTel-бакеты слишком грубые для in-memory сервиса |
| Нагрузочные тесты | `Taskfile.yaml` | ghz (gRPC) и vegeta (HTTP) со ступенчатой нагрузкой 50-500 RPS |

## Как запустить

```bash
task up              # Prometheus + Grafana + OTel Collector
task run:grpc        # терминал 1 -- gRPC сервер
task run:http        # терминал 2 -- HTTP сервер (опционально)
task load:grpc       # терминал 3 -- нагрузочный тест gRPC
task load:http       # терминал 3 -- нагрузочный тест HTTP
task test:api        # юнит-тесты
task down
```

Grafana: `localhost:3000` (admin/admin). RPS: `rate(rpc_server_call_duration_seconds_count[1m])`.

## Структура проекта

```
easy_rate_limiter/
├── cmd/
│   ├── grpc_server/        # gRPC с rate limiter interceptor + OTel
│   └── http_server/        # HTTP/Chi с rate limiter middleware + OTel
├── internal/
│   ├── ratelimit/          # limiter.go, grpc_interceptor.go, http_middleware.go
│   ├── api/                # обработчики: gRPC (grpc.go) и HTTP (http.go)
│   └── metrics/            # OTel MeterProvider (push-модель, 10s интервал)
├── docker-compose.yml      # Prometheus, Grafana, OTel Collector
└── tests/                  # 4 кейса: burst allowed/exceeded для gRPC и HTTP
```
