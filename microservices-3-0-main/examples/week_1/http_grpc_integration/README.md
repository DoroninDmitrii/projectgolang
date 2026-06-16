# HTTP + gRPC интеграция — межсервисное взаимодействие

Два сервиса: HTTP-фронтенд принимает REST-запросы и транслирует их в gRPC-вызовы бэкенда. Типичная схема в микросервисной архитектуре: REST наружу, gRPC внутри.

## Архитектура

```
curl/браузер
      |
      v
HTTP frontend (:8080)  ──gRPC──>  gRPC backend (:50051)
   Chi router                      UFO Service
   Handler с gRPC-клиентом         In-memory storage
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| gRPC-клиент как зависимость | `http_frontend/cmd/server/main.go` | Клиент создается при старте и передается в Handler |
| Keepalive на клиенте | `http_frontend/cmd/server/main.go` | Параметры поддержания соединения |
| Таймаут через контекст | `http_frontend/pkg/handler/` | `context.WithTimeout(r.Context(), 5s)` — propagation дедлайна |
| Маппинг gRPC -> HTTP ошибок | `http_frontend/pkg/handler/` | `status.FromError` + switch по `codes.*` |
| Go Workspace | `go.work` | Три модуля (frontend, backend, shared) в одном workspace |
| Shared proto | `shared/proto/ufo/v1/` | Общий контракт между сервисами |

## Маппинг ошибок

| gRPC | HTTP |
|------|------|
| NotFound | 404 |
| InvalidArgument | 400 |
| DeadlineExceeded | 504 |
| AlreadyExists | 409 |
| Unauthenticated | 401 |
| PermissionDenied | 403 |
| Internal | 500 |

## Как запустить

```bash
task proto:gen   # генерация кода
task run:grpc    # бэкенд на :50051
task run:http    # фронтенд на :8080 (в другом терминале)
task test:api    # API-тесты
```

## Структура проекта

```
http_grpc_integration/
├── grpc_backend/          # gRPC-сервис
│   └── cmd/server/
├── http_frontend/         # HTTP-сервис с gRPC-клиентом
│   ├── cmd/server/
│   └── pkg/handler/
├── shared/                # общие proto-определения
│   ├── proto/ufo/v1/
│   └── pkg/proto/         # сгенерированный код
├── tests/                 # API-тесты
├── go.work
└── Taskfile.yaml
```
