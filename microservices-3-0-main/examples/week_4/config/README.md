# Конфигурация микросервиса — YAML + env-переменные с приоритетами

Пример управления конфигурацией gRPC-сервиса для разных окружений (local, staging, production) с помощью библиотеки cleanenv.

## Концепция

Типичная проблема: сервис должен запускаться локально с одними настройками, на staging с другими, а в production секреты вообще нельзя хранить в файлах. Нужен единый механизм с чёткими приоритетами.

Решение — цепочка приоритетов:

```
CLI-флаг -config → env CONFIG_PATH → config.local.yaml (дефолт)
```

Внутри конфига ещё один уровень приоритетов:

```
системные env → .env файл → YAML-значения → env-default теги в структуре
```

Это позволяет:
- локально запускать без каких-либо переменных — всё берётся из YAML + дефолты
- на staging указать `CONFIG_PATH=config.staging.yaml` и перетереть пароли через env
- в production передать все секреты только через env, без файлов

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Цепочка приоритетов конфига | `ufo/internal/config/config.go` | `ResolveConfigPath()` — флаг > env > дефолт |
| YAML + env override | `ufo/internal/config/config.go` | `cleanenv.ReadConfig` читает YAML, затем перетирает env |
| Теги структуры | `ufo/internal/config/pg.go` | `yaml:"host" env:"POSTGRES_HOST" env-default:"localhost"` — три источника в одном поле |
| Профили окружений | `ufo/config.*.yaml` | local — localhost, staging — отдельный хост + SSL, production — только env |
| .env для локальной разработки | `ufo/cmd/grpc_server/main.go` | `godotenv.Load()` — опциональный, ошибка загрузки игнорируется |
| DI вручную в main | `ufo/cmd/grpc_server/main.go` | Прямая сборка: `repo → svc → api`, без фреймворков |
| Graceful shutdown | `ufo/cmd/grpc_server/main.go` | `GracefulStop()` с таймаутом и fallback на `Stop()` |
| Интеграционные тесты | `ufo/tests/api_test.go` | testcontainers + bufconn + goose-миграции |

## Архитектура

```
config.local.yaml ──┐
config.staging.yaml ─┼──→ cleanenv.ReadConfig ──→ Config struct ──→ main.go
config.production.yaml ┘         ↑
                            env-переменные
                            перетирают YAML
```

Конфиг-структура:

```go
type Config struct {
    GRPC grpcConfig `yaml:"grpc"`   // host, port
    PG   pgConfig   `yaml:"pg"`     // host, port, database, user, password, sslmode
}
```

## Как запустить

```bash
task up                           # PostgreSQL
task migrate:ufo:up               # Миграции
task run                          # Сервер (config.local.yaml по умолчанию)
task run -- -config config.staging.yaml  # Явный профиль
task test:api                     # Интеграционные тесты
task down                         # Остановка
```

## Структура проекта

```
config/
├── shared/proto/ufo/v1/          # Proto-определения
├── ufo/
│   ├── cmd/grpc_server/          # Точка входа, DI, graceful shutdown
│   ├── config.local.yaml         # Профили окружений
│   ├── config.staging.yaml
│   ├── config.production.yaml
│   ├── internal/
│   │   ├── config/               # Загрузка и структуры конфига
│   │   ├── api/ufo/v1/           # gRPC-обработчики
│   │   ├── service/ufo/          # Бизнес-логика
│   │   ├── repository/ufo/       # PostgreSQL-запросы
│   │   ├── model/                # Доменные модели
│   │   └── converter/            # Proto <-> модель
│   └── tests/                    # Интеграционные тесты
├── migrations/ufo/               # Goose-миграции
└── deploy/compose/               # Docker Compose (PostgreSQL)
```
