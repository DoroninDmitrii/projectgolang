# Tracing — распределённый трейсинг между микросервисами

Цепочка из трёх gRPC-сервисов с автоматической и ручной инструментацией OpenTelemetry. Визуализация в Jaeger.

## Концепция

Проблема: запрос проходит через несколько сервисов, и при ошибке или тормозах непонятно, где именно проблема.

Распределённый трейсинг решает это: каждый сервис создаёт span (отрезок работы), все спаны связаны единым trace ID. В Jaeger видно полное дерево вызовов с таймингами.

Как trace ID передаётся между сервисами:
1. Клиент создаёт trace ID и инжектит его в gRPC metadata (заголовок `traceparent`, W3C TraceContext)
2. Сервер извлекает trace ID из metadata и создаёт дочерний span
3. При исходящем вызове к другому сервису trace ID снова инжектится в metadata

В этом примере два уровня инструментации:
- **Автоматическая** — `otelgrpc.NewServerHandler()` / `NewClientHandler()` создают спаны для всех gRPC-вызовов без кода
- **Ручная (декоратор)** — сервисный слой обёрнут в tracing-декоратор, который добавляет бизнес-атрибуты (UUID, classification, confidence)

## Архитектура

```
Client
  ↓
UFO Service (:50051)  ──→  Analysis Service (:50052)  ──→  Classification Service (:50053)
  │  PostgreSQL              ↓                              чистая логика, без I/O
  │                    UFO Service.Get()
  │                    (обратный вызов для данных)
  │
  └── все сервисы ──→ OTel Collector (:4317) ──→ Jaeger (:16686)
```

Поток `AnalyzeSighting`:
1. UFO получает запрос, вызывает Analysis с UUID
2. Analysis вызывает UFO.Get() для получения данных и Classification для классификации
3. Classification определяет тип объекта по описанию, цвету, длительности
4. Результат возвращается по цепочке

Все 4+ спанов связаны одним trace ID и видны как дерево в Jaeger.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| InitTracer | `platform/pkg/tracing/provider.go` | OTLP exporter → Resource (service.name, version, environment) → ParentBased sampler → TracerProvider |
| Propagation | `platform/pkg/tracing/provider.go` | `W3C TraceContext + Baggage` — стандартные propagators для межсервисного контекста |
| Auto-instrumentation | `ufo/internal/app/app.go` | `otelgrpc.NewServerHandler()` — автоспаны для всех RPC без кода. На клиенте аналогично `NewClientHandler()` |
| TraceID в ответе | `platform/pkg/tracing/interceptor.go` | `TraceIDUnaryServerInterceptor` — добавляет trace ID в gRPC response headers для дебага |
| Tracing-декоратор (UFO) | `ufo/internal/service/ufo/tracing/tracing.go` | Оборачивает сервис, добавляет span с атрибутами `ufo.uuid`, `analysis.classification`, `analysis.confidence` |
| Tracing-декоратор (Analysis) | `analysis/internal/service/analysis/tracing/tracing.go` | Span с атрибутами бизнес-результата |
| Tracing-декоратор (Classification) | `classification/internal/service/classification/tracing/tracing.go` | Span с атрибутами классификации |
| DI с декоратором | `ufo/internal/app/di.go` | `NewTracedService(svc)` — сервис оборачивается при сборке, бизнес-логика не знает про трейсинг |
| Closer (LIFO) | `platform/pkg/closer/closer.go` | Graceful shutdown в обратном порядке: сервер → flush спанов → БД |
| Конфиг коллектора | `otel-collector-config.yaml` | `probabilistic_sampler` перед `batch` — сначала отбросить лишние спаны, потом батчировать |
| gRPC клиенты | `analysis/internal/client/grpc/` | `otelgrpc.NewClientHandler()` для автоматической propagation trace context |

Паттерн декоратора: бизнес-сервис реализует интерфейс, tracing-обёртка реализует тот же интерфейс, делегируя вызовы и добавляя span. Это позволяет добавить/убрать трейсинг без изменения бизнес-логики.

## Как запустить

```bash
task up-all                        # Jaeger + OTel Collector + PostgreSQL
task env:generate-all              # генерация .env файлов (если нужно)
```

Запуск сервисов (в отдельных терминалах):

```bash
# UFO Service
cd ufo && set -a && source ../deploy/compose/ufo/.env && set +a && go run cmd/grpc_server/main.go

# Analysis Service
cd analysis && go run cmd/main.go

# Classification Service
cd classification && go run cmd/main.go
```

Тестирование:

```bash
task tracing:test:full-cycle       # создание + анализ 3 объектов
task tracing:check-services        # проверка сервисов в Jaeger
task test:api                      # интеграционные тесты (testcontainers)
task down-all                      # остановка
```

Jaeger UI: http://localhost:16686 — выбрать сервис "ufo-service", найти трейсы.

## Структура проекта

```
tracing/
├── shared/proto/                     # общие proto-определения (ufo, analysis, classification)
├── platform/pkg/
│   ├── tracing/                      # InitTracer, interceptor, context utils
│   ├── logger/                       # slog
│   ├── closer/                       # graceful shutdown (LIFO)
│   └── grpc/health/                  # health check
├── ufo/                              # UFO Service (:50051)
│   ├── internal/service/ufo/tracing/ # tracing-декоратор
│   ├── internal/app/                 # DI, инициализация
│   └── migrations/                   # PostgreSQL
├── analysis/                         # Analysis Service (:50052)
│   └── internal/service/analysis/tracing/
├── classification/                   # Classification Service (:50053)
│   └── internal/service/classification/tracing/
├── deploy/compose/
│   ├── core/                         # Jaeger + OTel Collector
│   └── ufo/                          # PostgreSQL
└── otel-collector-config.yaml
```
