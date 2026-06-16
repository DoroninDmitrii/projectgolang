# Session Service — регистрация, логин и управление сессиями

gRPC-сервис с полным жизненным циклом сессии: Register -> Login -> Whoami -> Logout. Пароли хешируются bcrypt, сессии хранятся in-memory.

## На что обратить внимание

| Что | Файл |
|-----|------|
| Register/Login/Whoami/Logout с in-memory хранилищем, защищённым `sync.RWMutex` | `internal/server/server.go` |
| Хеширование паролей через `bcrypt.GenerateFromPassword` / `CompareHashAndPassword` | `internal/server/server.go:Register(), Login()` |
| Interceptor с whitelist: Register и Login публичные, Whoami и Logout требуют токен | `internal/interceptor/auth.go` |
| Bearer-токен из `authorization` metadata, `strings.CutPrefix` для парсинга | `internal/interceptor/auth.go:Auth()` |
| Whitelist через сгенерированные константы `SessionService_*_FullMethodName` | `internal/interceptor/auth.go:publicMethods` |
| Идемпотентный Logout — `delete(s.sessions, token)` не падает на отсутствующем ключе | `internal/server/server.go:Logout()` |

## Как запустить

```bash
go run cmd/grpc_server/main.go

# Регистрация
grpcurl -plaintext -d '{"email":"a@b.com","password":"123","name":"Test"}' \
  localhost:50051 session.v1.SessionService/Register

# Логин — получаем session_token
grpcurl -plaintext -d '{"email":"a@b.com","password":"123"}' \
  localhost:50051 session.v1.SessionService/Login

# Whoami — передаём токен через metadata
grpcurl -plaintext -H 'authorization: Bearer <token>' \
  localhost:50051 session.v1.SessionService/Whoami
```

## Структура проекта

```
easy_session/
├── cmd/grpc_server/       # Точка входа
├── internal/
│   ├── server/            # Реализация SessionService
│   └── interceptor/       # Auth interceptor (Bearer token)
├── proto/session/v1/      # Proto-определения
└── tests/                 # Интеграционные тесты (bufconn)
```
