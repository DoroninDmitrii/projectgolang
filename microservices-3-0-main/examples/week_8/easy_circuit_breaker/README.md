# Circuit Breaker -- защита от каскадных сбоев

Пример клиентского Circuit Breaker через `sony/gobreaker/v2`. При серии сбоев клиент перестаёт отправлять запросы на сервер, давая ему время восстановиться. После таймаута пробный запрос проверяет, ожил ли сервер.

## Концепция

Circuit Breaker -- конечный автомат с тремя состояниями:

```
         ошибки >= 3 подряд              timeout (5 сек)
  Closed ───────────────────► Open ────────────────────► HalfOpen
    ▲                                                      │
    │              все пробные запросы успешны              │
    └──────────────────────────────────────────────────────┘
                          │  хотя бы один сбой  │
                          │                     ▼
                          └──────────────── Open
```

- **Closed** -- запросы проходят на сервер, ошибки считаются
- **Open** -- запросы мгновенно отклоняются без вызова сервера (fail fast)
- **HalfOpen** -- пропускается 1 пробный запрос для проверки восстановления

Главная задача -- **классификация ошибок**. Бизнес-ошибки (404, 400) не должны открывать circuit breaker: клиент шлёт невалидные данные, но сервер при этом работает нормально. Считать их сбоями -- ложное срабатывание.

## Архитектура

```
gRPC Client
    │
    ├─ Circuit Breaker Interceptor (клиентская сторона)
    │     ├─ Closed: invoker → сервер
    │     ├─ Open: return Unavailable (сервер не вызывается)
    │     └─ HalfOpen: invoker → сервер (пробный запрос)
    │
    └─► gRPC Server (OTel metrics → Collector → Prometheus → Grafana)
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Конфигурация CB | `cmd/grpc_client/main.go:34-85` | Все параметры gobreaker с комментариями: MaxRequests, Interval, Timeout, ReadyToTrip |
| Стратегии ReadyToTrip | `cmd/grpc_client/main.go:66-74` | Три варианта: ConsecutiveFailures, TotalFailures, error rate |
| Классификация ошибок | `internal/circuitbreaker/interceptor.go:27-43` | `isBusinessError` -- какие коды НЕ считать сбоями |
| Обёртка invoker в Execute | `internal/circuitbreaker/interceptor.go:72-86` | Бизнес-ошибка сохраняется в замыкании, для gobreaker возвращается nil |
| Приоритет возврата ошибок | `internal/circuitbreaker/interceptor.go:90-106` | originalErr > ErrOpenState > cbErr > nil |
| SetFailMode для управления | `internal/api/grpc.go` | Отдельный RPC для включения/выключения сбоев, всегда возвращает OK |
| Демо полного цикла | `cmd/grpc_client/main.go:104-163` | Closed → Open → HalfOpen → Closed с логированием переходов |

## Как запустить

```bash
task up             # Prometheus + Grafana + OTel Collector
task run:grpc       # терминал 1 -- gRPC сервер
task run:client     # терминал 2 -- демонстрация полного цикла CB
task test:api       # тесты
task down
```

Grafana: `localhost:3000` (admin/admin). Метрики `rpc.server.call.duration` показывают статусы OK / Internal / Unavailable.

## Структура проекта

```
easy_circuit_breaker/
├── cmd/
│   ├── grpc_server/              # сервер с SetFailMode и OTel метриками
│   └── grpc_client/              # клиент с gobreaker и демо-сценарием
├── internal/
│   ├── circuitbreaker/           # interceptor с классификацией ошибок
│   ├── api/                      # обработчики + failMode через atomic.Bool
│   └── metrics/                  # OTel MeterProvider (push-модель)
├── grafana/provisioning/         # автоконфигурация datasource и dashboard
├── docker-compose.yml            # Prometheus, Grafana, OTel Collector
└── tests/                        # 4 кейса: closed, open, бизнес-ошибки, recovery
```
