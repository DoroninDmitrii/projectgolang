# Easy Logs — централизованное логирование через OpenTelemetry

Сбор структурированных логов из Go-приложения в Elasticsearch через OpenTelemetry Collector. Визуализация в Kibana.

## Концепция

Проблема: в распределённой системе логи раскиданы по stdout каждого контейнера. Искать по ним — мучение.

Решение: все сервисы отправляют логи по единому протоколу (OTLP) в централизованное хранилище. OpenTelemetry Collector работает как агент — принимает, обогащает метаданными, батчирует и доставляет в хранилище.

Ключевые идеи:
- **Tee-паттерн (fanout)** — логи пишутся одновременно в stdout и в OTLP. Если коллектор недоступен, приложение продолжает работать со stdout.
- **Единая схема** — логи маппятся в Elastic Common Schema (ECS), что даёт стандартные поля для поиска (`service.name`, `event.severity_text`).
- **Метаданные** — `service.name` задаёт приложение, `deployment.environment` добавляет коллектор. Сервис не знает про окружение.

## Архитектура

```
Go App (slog)
  ├── stdout (JSON)
  └── OTLP gRPC (:4317)
        ↓
  OTel Collector
    processors: memory_limiter → resource → transform → batch
        ↓
  Elasticsearch (:9200)     ← индекс "easy-logs", ECS mapping
        ↓
  Kibana (:5601)            ← Data View "easy-logs*"
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Tee-handler (fanout) | `internal/logger/logger.go` | `slogmulti.Fanout()` — дублирует запись в stdout и OTLP. Graceful degradation при недоступности коллектора |
| OTLP Log Provider | `internal/logger/logger.go` | Инициализация OTel SDK: exporter → resource (service.name) → BatchProcessor → otelslog bridge |
| gRPC LoggingInterceptor | `internal/interceptor/logging.go` | Логирует метод, duration, gRPC status code. Использует `slog.LogAttrs()` без аллокаций |
| Init / Close | `cmd/grpc_server/main.go` | `Init("info", true)` + `defer Close()` — flush буфера перед выходом |
| Конфиг коллектора | `otel-collector-config.yaml` | Порядок процессоров: memory_limiter первый (защита от OOM), batch последний |
| Тесты с testcontainers | `tests/api_test.go` | Поднимает ES + OTel Collector в Docker, проверяет сквозную доставку логов |

Trade-off: `sync.Once` в `Init()` + глобальный `slog.SetDefault()` — простота в ущерб тестируемости. Для учебного примера это ок, в проде лучше передавать логгер через DI.

## Как запустить

```bash
task up              # Elasticsearch + Kibana + OTel Collector
task run             # gRPC сервер (:50051) с логированием
task test:get        # тестовый gRPC-запрос для генерации логов
task test:api        # интеграционные тесты (testcontainers)
task down            # остановка
```

Kibana: http://localhost:5601 — Data View создаётся автоматически.

KQL-запросы для поиска:

```
service.name: "ufo-service" AND event.severity_text: "ERROR"
message: "gRPC метод завершён"
```

## Структура проекта

```
easy_logs/
├── cmd/grpc_server/          # точка входа
├── internal/
│   ├── logger/               # Init/Close, fanout handler, OTLP bridge
│   ├── interceptor/          # gRPC logging interceptor
│   └── handler/              # gRPC handlers (fake data через gofakeit)
├── tests/                    # интеграционные тесты (testcontainers)
├── proto/ufo/v1/             # proto-определения
├── docker-compose.yml        # ES + Kibana + OTel Collector
└── otel-collector-config.yaml
```
