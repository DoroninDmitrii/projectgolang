# Session Auth — проброс токена из HTTP в gRPC

Два сервиса: HTTP-фронтенд (Chi) принимает REST-запросы с Bearer-токеном и проксирует их в gRPC-бэкенд. Бэкенд валидирует сессию через interceptor и добавляет пользователя в контекст.

## Архитектура

```
Client                    HTTP Frontend (:8080)              gRPC Backend (:50051)
  │                              │                                  │
  │  Authorization: Bearer xxx   │                                  │
  │ ──────────────────────────>  │                                  │
  │                              │  session-token в gRPC metadata   │
  │                              │ ──────────────────────────────>  │
  │                              │                                  │  interceptor: token -> User
  │                              │                                  │  handler: UserFromContext()
  │          HTTP JSON           │          gRPC response           │
  │ <──────────────────────────  │ <──────────────────────────────  │
```

## На что обратить внимание

| Что | Файл |
|-----|------|
| HTTP middleware: извлекает Bearer-токен, кладёт в контекст (не валидирует) | `http_frontend/pkg/middleware/http_auth.go` |
| gRPC interceptor: читает token из incoming metadata, проверяет сессию, кладёт User в контекст | `grpc_backend/pkg/interceptor/grpc_auth.go` |
| Проброс токена: HTTP context -> gRPC outgoing metadata через `metadata.AppendToOutgoingContext` | `http_frontend/pkg/handler/handler.go:withGRPCAuth()` |
| Incoming vs Outgoing metadata: подробный комментарий почему они разделены | `grpc_backend/pkg/interceptor/grpc_auth.go` |
| Context keys через `int` + `iota` вместо строк, с пояснением про linked list внутри context | `shared/pkg/auth/context.go` |
| Маппинг gRPC codes -> HTTP status codes | `http_frontend/pkg/handler/handler.go:handleGRPCError()` |
| Whitelist публичных методов (Reflection API) | `grpc_backend/pkg/interceptor/grpc_auth.go:publicMethods` |

## Как запустить

```bash
# Терминал 1: gRPC-бэкенд
go run grpc_backend/cmd/server/main.go

# Терминал 2: HTTP-фронтенд
go run http_frontend/cmd/server/main.go

# Терминал 3: запросы
curl -H 'Authorization: Bearer token-admin-123' http://localhost:8080/api/v1/sightings
```

## Структура проекта

```
easy_auth/
├── http_frontend/
│   ├── cmd/server/          # HTTP-сервер (Chi + gRPC клиент)
│   └── pkg/
│       ├── handler/         # REST -> gRPC проксирование
│       └── middleware/      # Извлечение Bearer-токена
├── grpc_backend/
│   ├── cmd/server/          # gRPC-сервер
│   └── pkg/
│       ├── interceptor/     # Валидация сессии
│       ├── service/         # Бизнес-логика
│       └── session/         # In-memory хранилище сессий
└── shared/
    ├── pkg/auth/            # User, context helpers
    └── proto/               # Proto-определения UFOService
```
