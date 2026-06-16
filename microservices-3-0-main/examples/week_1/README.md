# Week 1 — HTTP и gRPC

Основы построения API-сервисов на Go: два ключевых подхода (HTTP и gRPC), от простого CRUD до межсервисного взаимодействия и multi-module проекта.

## Путь обучения

Примеры расположены в порядке нарастания сложности:

| # | Директория | Что демонстрирует |
|---|-----------|-------------------|
| 1 | `grpc/` | gRPC, Protocol Buffers, CRUD, in-memory хранилище |
| 2 | `http_chi/` | REST API с Chi, middleware |
| 3 | `http_chi_ogen/` | Contract-first: кодогенерация из OpenAPI через Ogen |
| 4 | `grpc_with_interceptor/` | gRPC interceptors: логирование и recovery |
| 5 | `grpc_gateway_swagger_validation/` | gRPC-Gateway + REST + Swagger UI + валидация |
| 6 | `http_grpc_integration/` | HTTP frontend вызывает gRPC backend |
| 7 | `workspace/` | Go Workspaces, multi-module монорепо |

## Общие компоненты

[testutil/](testutil/) — переиспользуемые утилиты для API-тестов: создание клиентов, fixtures с Builder-паттерном, assertions для gRPC/HTTP статусов.

## Дополнительные материалы

- [GRPC_CONNECTIONS.md](GRPC_CONNECTIONS.md) — жизненный цикл gRPC соединений, keepalive, GOAWAY
- [HTTP_SERVER.md](HTTP_SERVER.md) — таймауты HTTP-сервера, защита от Slowloris, graceful shutdown
