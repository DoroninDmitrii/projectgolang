# gRPC — базовый CRUD-сервис

Фундаментальный пример: gRPC-сервис для управления наблюдениями НЛО. Знакомит с Protocol Buffers, кодогенерацией и основными паттернами работы с gRPC в Go.

## Концепция

gRPC — фреймворк удаленного вызова процедур поверх HTTP/2. API описывается в `.proto`-файлах, из которых генерируется код сервера и клиента. В отличие от REST, контракт строго типизирован и не зависит от языка реализации.

Подробнее о жизненном цикле gRPC-соединений — в [GRPC_CONNECTIONS.md](../GRPC_CONNECTIONS.md).

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Proto-определение сервиса | `proto/ufo/v1/ufo.proto` | Контракт API: RPC-методы, сообщения, nullable-поля через `wrappers` |
| Partial Update | `proto/ufo/v1/ufo.proto` | `SightingUpdateInfo` — все поля optional для частичного обновления |
| Keepalive-параметры | `cmd/grpc_server/main.go` | MaxConnectionIdle, MaxConnectionAge — ротация соединений |
| In-memory хранилище | `cmd/grpc_server/main.go` | `sync.RWMutex` + `map` + `proto.Clone` для потокобезопасности |
| Soft delete | `cmd/grpc_server/main.go` | Delete проставляет `deleted_at`, а не удаляет запись |
| Клиент для тестирования | `cmd/grpc_client/main.go` | Полный цикл CRUD через сгенерированный клиент |

## Как запустить

```bash
task proto:gen   # генерация Go-кода из proto
task run         # сервер на :50051
```

В другом терминале:

```bash
go run cmd/grpc_client/main.go
```

## Структура проекта

```
grpc/
├── cmd/
│   ├── grpc_server/   # сервер
│   └── grpc_client/   # клиент для тестирования
├── proto/ufo/v1/      # proto-определения
├── pkg/proto/         # сгенерированный код
└── Taskfile.yaml
```
