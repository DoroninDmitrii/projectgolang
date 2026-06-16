# Easy Prometheus — метрики через OpenTelemetry (push-модель)

Сбор метрик из Go-приложения по протоколу OTLP в Prometheus через OTel Collector. Визуализация в Grafana с готовым дашбордом.

## Концепция

Классический Prometheus работает в pull-модели: сам ходит по эндпоинтам `/metrics`. Это требует service discovery и сетевой доступности.

Push-модель через OTel: приложение само отправляет метрики в коллектор по OTLP gRPC, а коллектор пишет в Prometheus через Remote Write API. Преимущества:
- Сервису не нужен HTTP-эндпоинт для метрик
- Работает за NAT и в serverless
- Единый протокол OTLP для логов, метрик и трейсов

Три типа метрик в OTel:
- **Counter** — монотонно растёт (создано наблюдений)
- **UpDownCounter** — растёт и убывает (активных наблюдений)
- **Histogram** — распределение значений по бакетам (latency)

## Архитектура

```
Go App (OTel SDK)
  ├── otelgrpc.StatsHandler     ← автоматические gRPC метрики
  └── custom metrics            ← бизнес-метрики (counters, histograms)
        ↓ OTLP gRPC (:4317)
  OTel Collector
    processors: memory_limiter → resource → batch
        ↓ Prometheus Remote Write
  Prometheus (:9091)
        ↓
  Grafana (:3000)               ← готовый dashboard
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Инициализация MeterProvider | `internal/metrics/metrics.go` | OTLP exporter → PeriodicReader (10s) → кастомные бакеты гистограммы. `Views` для переопределения бакетов |
| Три типа инструментов | `internal/metrics/metrics.go` | `Int64Counter`, `Int64UpDownCounter`, `Float64Histogram` — разные семантики |
| Dual instrumentation | `cmd/grpc_server/main.go` | `otelgrpc.NewServerHandler()` для автоматических gRPC-метрик + `MetricsInterceptor()` для кастомных |
| Бизнес-метрики в handler | `internal/handler/ufo.go` | `SightingsCreatedTotal.Add(ctx, 1)` при Create, `SightingsActive.Add(ctx, -1)` при Delete |
| Interceptor для latency | `internal/interceptor/metrics.go` | Записывает duration каждого RPC в histogram с атрибутом `rpc.method` |
| Grafana dashboard | `grafana/provisioning/dashboards/ufo-service.json` | 5 панелей: RPS, P95, custom histogram, counters. Автопровизия при старте |
| Тесты метрик e2e | `tests/api_test.go` | Поднимает Prometheus + Collector, делает вызовы, проверяет метрики через Prometheus HTTP API |

Trade-off: интервал экспорта 10s (вместо дефолтных 60s) — быстрая обратная связь при разработке, в проде вернуть 60s.

## Как запустить

```bash
task up              # Prometheus + Grafana + OTel Collector
task run             # gRPC сервер (:50051)
task load            # нагрузочный тест ghz (50→500 RPS, 2.5 мин)
task test:api        # интеграционные тесты (testcontainers)
task down            # остановка
```

Grafana: http://localhost:3000 — dashboard "UFO Service" доступен сразу.

Полезные PromQL:

```promql
# RPS по методам
sum(rate(rpc_server_call_duration_seconds_count[1m])) by (rpc_method)

# P95 latency
histogram_quantile(0.95, sum(rate(rpc_server_call_duration_seconds_bucket[5m])) by (le))

# Бизнес-метрики
ufo_service_sightings_active
ufo_service_sightings_created_total
```

## Структура проекта

```
easy_prometheus/
├── cmd/grpc_server/              # точка входа
├── internal/
│   ├── metrics/                  # MeterProvider, instruments (counter, histogram)
│   ├── interceptor/              # MetricsInterceptor (latency histogram)
│   └── handler/                  # gRPC handlers с бизнес-метриками
├── tests/                        # e2e тесты метрик (testcontainers)
├── grafana/provisioning/         # datasource + dashboard (auto-provisioned)
├── proto/ufo/v1/                 # proto-определения
├── docker-compose.yml            # Prometheus + Grafana + OTel Collector
└── otel-collector-config.yaml
```
