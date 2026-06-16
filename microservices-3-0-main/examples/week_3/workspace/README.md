# Workspace — мультисервисная архитектура с кодогенерацией

Два микросервиса (gRPC + HTTP) в одном Go workspace с общими спецификациями и кодогенерацией из Protocol Buffers и OpenAPI.

## Архитектура

```
                    shared/
          ┌─────── proto/ufo/v1/ ──── buf generate ──── pkg/proto/
          │         api/weather/v1/ ── ogen ──────────── pkg/openapi/
          │
    ┌─────┴──────┐                          ┌────────────────┐
    │ gRPC       │                          │ HTTP           │
    │ :50051     │                          │ :8080          │
    │            │                          │                │
    │ UFO CRUD   │                          │ Weather API    │
    │ pgx + PG   │                          │ Chi + Ogen     │
    └────────────┘                          │ pgx + PG       │
          │                                 └────────────────┘
          │                                        │
          ▼                                        ▼
    ┌────────────┐                          ┌────────────┐
    │ PostgreSQL │                          │ PostgreSQL │
    │ (grpc)     │                          │ (http)     │
    └────────────┘                          └────────────┘
```

Оба сервиса используют `go.work` для ссылки на общий модуль `shared/` с сгенерированным кодом.

## Концепция

### Go workspace

Монорепо с несколькими Go-модулями. `go.work` связывает модули `shared`, `grpc`, `http` — каждый со своим `go.mod`, но импорты между ними работают без `replace` директив.

### Кодогенерация

**gRPC (buf)** — из `.proto` файлов генерируются Go-интерфейсы сервера, клиент и типы сообщений. Сервис реализует сгенерированный интерфейс.

**HTTP (ogen)** — из OpenAPI YAML генерируются типизированные хендлеры, модели запросов/ответов, валидация. Chi используется как роутер, Ogen подключается к нему как `http.Handler`.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Go workspace | `go.work` | Связывает три модуля без `replace` |
| gRPC reflection | `grpc/cmd/server/main.go` | `reflection.Register` — позволяет grpcurl работать без proto-файлов |
| Keepalive | `grpc/cmd/server/main.go` | `MaxConnectionIdle`, `MaxConnectionAge` — защита от утечки соединений |
| Ogen + Chi интеграция | `http/cmd/server/main.go` | Ogen-сервер монтируется в Chi как subrouter |
| Custom middleware | `http/internal/middleware/` | `RequestLogger` — логирование HTTP-запросов |
| Converter-слой | `grpc/internal/api/converter/`, `http/internal/api/converter/` | Преобразование между транспортными DTO (proto/ogen) и доменной моделью |
| Раздельный deploy | `deploy/compose/` | У каждого сервиса свой Compose-файл и своя БД |
| Proto lint rules | `shared/proto/buf.yaml` | STANDARD + COMMENT_* — все proto-сущности требуют документации |

## Как запустить

```bash
# Генерация кода (при изменении спецификаций)
task gen

# Поднять инфраструктуру
task up-all

# Применить миграции
task migrate:grpc:up
task migrate:http:up

# Запустить сервисы
task run:grpc    # gRPC на :50051
task run:http    # HTTP на :8080

# Тесты
task test:api

# Остановить
task down-all
```

## Структура проекта

```
workspace/
├── shared/
│   ├── proto/ufo/v1/              # Proto-спецификация gRPC API
│   ├── api/weather/v1/            # OpenAPI-спецификация HTTP API
│   └── pkg/                       # Сгенерированный код
├── grpc/
│   ├── cmd/server/                # Точка входа gRPC-сервера
│   └── internal/                  # api → converter → repository
├── http/
│   ├── cmd/server/                # Точка входа HTTP-сервера
│   └── internal/                  # api → middleware → converter → repository
├── deploy/compose/                # Docker Compose для каждого сервиса
├── go.work                        # Go workspace
└── Taskfile.yaml
```
