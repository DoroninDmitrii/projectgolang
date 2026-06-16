# Docker — multi-stage build Go-приложения

Сборка и запуск Go HTTP-сервера в Docker-контейнере. Показывает, как получить минимальный и безопасный production-образ.

## Концепция

### Multi-stage build

Обычная сборка Go в Docker тянет за собой весь SDK (~1 ГБ) в финальный образ. Multi-stage build решает проблему: первая стадия компилирует бинарник в тяжёлом образе с Go SDK, вторая — копирует только готовый бинарник в минимальный Alpine (~5 МБ).

```
┌───────────────────────┐      ┌──────────────────────┐
│  Stage 1: builder     │      │  Stage 2: runtime    │
│  golang:1.26-alpine   │      │  alpine:3.23         │
│                       │      │                      │
│  go mod download      │      │  COPY --from=builder │
│  go build -o server   │─────▶│  ./server            │
│                       │      │                      │
│  ~1 ГБ               │      │  ~10 МБ              │
└───────────────────────┘      └──────────────────────┘
```

### Статическая линковка

Alpine использует musl вместо glibc. Чтобы бинарник работал без зависимостей от C-библиотек, нужна статическая линковка: `CGO_ENABLED=0 GOOS=linux`.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Multi-stage build | `Dockerfile` | Две стадии: builder на Go SDK, runtime на Alpine |
| Статическая линковка | `Dockerfile:19` | `CGO_ENABLED=0` — бинарник не зависит от libc |
| Non-root пользователь | `Dockerfile:30-39` | `adduser -S appuser` — контейнер не работает от root |
| Кеширование слоёв | `Dockerfile:10-13` | `go.mod` и `go.sum` копируются отдельно — `go mod download` кешируется между сборками |
| Connection pooling | `main.go:42` | `pgxpool.New` — пул соединений вместо одного подключения |
| Graceful shutdown | `main.go:82-97` | Перехват SIGINT/SIGTERM, `server.Shutdown` с таймаутом |
| HTTP таймауты | `main.go:65-71` | ReadTimeout, WriteTimeout, IdleTimeout — защита от slowloris |

## Как запустить

```bash
# Собрать образ
task docker:build

# Запустить контейнер
task docker:run
```

Для работы с базой нужно поднять PostgreSQL отдельно и создать таблицу `users` (см. пример `docker_compose`).

## Структура проекта

```
docker/
├── main.go              # HTTP-сервер с эндпоинтом POST /users
├── Dockerfile           # Multi-stage build
├── .dockerignore        # Исключения при сборке
└── Taskfile.yaml
```
