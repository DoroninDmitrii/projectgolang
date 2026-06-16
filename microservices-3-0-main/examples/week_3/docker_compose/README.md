# Docker Compose — оркестрация Go-приложения с PostgreSQL

Автоматизация запуска нескольких контейнеров одной командой. Развивает пример `docker/`, добавляя Compose, healthcheck и миграции.

## Концепция

Без Docker Compose каждый контейнер запускается вручную: отдельно БД, отдельно приложение, между ними — ручная настройка сети, volumes, переменных окружения. Compose описывает всю инфраструктуру в одном `docker-compose.yml` и управляет ей одной командой.

```
docker compose up --build
       │
       ▼
┌──────────────┐     healthcheck      ┌──────────────┐
│  PostgreSQL  │◄─────pg_isready──────│  Docker      │
│  :5432       │     interval=10s     │  Engine      │
└──────┬───────┘                      └──────┬───────┘
       │  service_healthy                    │
       │                                     ▼
       │                              ┌──────────────┐
       └──────────bridge──────────────│  Go App      │
              app-network             │  :8080       │
                                      └──────────────┘
```

### Healthcheck и depends_on

`depends_on` без condition не гарантирует, что БД готова принимать соединения — только что контейнер запущен. `condition: service_healthy` + `pg_isready` решает проблему: приложение стартует только когда Postgres действительно принимает подключения.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Healthcheck PostgreSQL | `docker-compose.yml:12-16` | `pg_isready` проверяет готовность, а не просто запуск контейнера |
| `condition: service_healthy` | `docker-compose.yml:33` | Приложение стартует только после готовности БД |
| Переменные из `.env` | `docker-compose.yml` | `${POSTGRES_USER}` — подстановка из `.env` файла |
| Named volume | `docker-compose.yml:44` | `pgdata` — данные переживают пересоздание контейнера |
| Конфиг из env vars | `cmd/main.go` | `os.Getenv("DB_URI")` вместо хардкода (ср. с `docker/main.go`) |
| Goose-миграции | `migrations/` | Формат `-- +goose Up` / `-- +goose Down` для версионирования схемы |

## Как запустить

```bash
# Поднять PostgreSQL + приложение
task docker:compose:up

# Применить миграции
task migrate:up

# Проверить
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "email": "alice@example.com"}'

# Остановить
task docker:compose:down
```

## Структура проекта

```
docker_compose/
├── cmd/
│   └── main.go                    # HTTP-сервер, конфиг из env vars
├── migrations/
│   └── 001_create_user_table.sql  # Goose-миграция
├── docker-compose.yml             # PostgreSQL + приложение
├── Dockerfile                     # Multi-stage build
├── .env                           # Переменные окружения
└── Taskfile.yaml
```
