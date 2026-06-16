# gRPC Interceptors — middleware для gRPC

Расширение базового gRPC-сервиса из `grpc/` интерцепторами — аналогом HTTP middleware. Реализованы логирование запросов и перехват паник.

## Концепция

Interceptor — функция, которая оборачивает вызов gRPC-метода. Получает запрос до handler'а и ответ после. Используется для сквозной функциональности: логирование, метрики, аутентификация, tracing, recovery.

Два типа:
- **Unary** — для обычных запрос-ответ вызовов (реализованы в этом примере)
- **Stream** — для потоковых вызовов

Interceptors подключаются цепочкой через `grpc.ChainUnaryInterceptor`. Порядок важен: Recovery должен быть первым, чтобы перехватить паники из всех последующих interceptors.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Сигнатура interceptor | `internal/interceptor/logger.go` | `grpc.UnaryServerInterceptor` — принимает ctx, req, info, handler |
| Извлечение имени метода | `internal/interceptor/logger.go` | Парсинг `info.FullMethod` для логирования |
| Измерение времени | `internal/interceptor/logger.go` | `time.Since` для замера длительности вызова |
| Recovery от паник | `internal/interceptor/recovery.go` | `defer/recover` + `debug.Stack()` для stack trace |
| Конвертация паники в gRPC-ошибку | `internal/interceptor/recovery.go` | Паника превращается в `codes.Internal`, а не роняет сервер |
| Порядок цепочки | `cmd/grpc_server/main.go` | Recovery первым, Logger вторым |

## Как запустить

```bash
task proto:gen   # генерация кода
task run         # сервер на :50051
task test:api    # API-тесты
```

## Структура проекта

```
grpc_with_interceptor/
├── cmd/
│   ├── grpc_server/       # сервер с interceptors
│   └── grpc_client/       # клиент
├── internal/interceptor/  # реализация interceptors
│   ├── logger.go          # логирование
│   └── recovery.go        # перехват паник
├── proto/ufo/v1/          # proto-определения
├── pkg/proto/             # сгенерированный код
├── tests/                 # API-тесты
└── Taskfile.yaml
```
