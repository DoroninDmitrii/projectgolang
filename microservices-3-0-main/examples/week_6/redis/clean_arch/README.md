# Clean Architecture + Redis Cache — cache-aside паттерн

Сервис наблюдений НЛО с чистой архитектурой, PostgreSQL как основное хранилище и Redis как кеш. Фокус примера — интеграция кеширования в слоёную архитектуру.

## Архитектура

```
gRPC Request
    │
    ▼
API Layer (converter: proto <-> domain)
    │
    ▼
Service Layer ──── cache-aside ────> Cache Repository (Redis)
    │                                     │
    │  cache miss                         │ cache hit
    ▼                                     │
UFO Repository (PostgreSQL) ──────────────┘
                               warm cache after DB read
```

## На что обратить внимание

| Что | Файл |
|-----|------|
| Cache-aside в service: кеш -> БД -> прогрев кеша. Ошибка записи в кеш не ломает операцию | `ufo/internal/service/ufo/get.go` |
| Интерфейсы репозиториев определены в service layer (Dependency Inversion) | `ufo/internal/service/ufo/deps.go` |
| Отдельные модели для каждого слоя: proto, domain (`model/`), Redis (`repository/model/redis_view.go`), PG (`repository/model/pg_view.go`) | `ufo/internal/model/`, `ufo/internal/repository/model/` |
| Конвертеры между слоями: proto <-> domain и domain <-> repository | `ufo/internal/converter/`, `ufo/internal/repository/converter/` |
| Redis pipeline: HSET + EXPIRE в одном round-trip | `ufo/internal/repository/ufo_cache/set.go` |
| DI без фреймворков: lazy initialization с проверкой на nil | `ufo/internal/app/di.go` |
| Closer pattern: стек cleanup-функций для graceful shutdown | `platform/pkg/closer/closer.go` |
| Soft delete через `deleted_at` timestamp | `ufo/internal/repository/ufo/delete.go` |
| Маппинг domain errors -> gRPC codes | `ufo/internal/api/ufo/v1/get.go` |

## Как запустить

```bash
task up                              # PostgreSQL + Redis
go run ufo/cmd/grpc_server/main.go   # Запустить сервер
task down                            # Остановить всё
```

## Структура проекта

```
clean_arch/
├── ufo/                          # Сервис
│   ├── cmd/grpc_server/          # Точка входа
│   └── internal/
│       ├── api/ufo/v1/           # gRPC handlers
│       ├── service/ufo/          # Бизнес-логика + cache-aside
│       ├── repository/ufo/       # PostgreSQL
│       ├── repository/ufo_cache/ # Redis
│       ├── repository/model/     # PG и Redis view models
│       ├── repository/converter/ # domain <-> repository models
│       ├── converter/            # proto <-> domain models
│       ├── model/                # Domain entities
│       ├── config/               # Конфигурация (cleanenv)
│       ├── app/                  # DI container + lifecycle
│       └── errors/               # Domain errors
├── shared/proto/                 # Proto-определения
├── platform/pkg/                 # Общие утилиты (closer, logger, Redis client)
└── deploy/compose/               # Docker Compose (PG + Redis)
```
