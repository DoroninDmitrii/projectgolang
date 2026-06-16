# gRPC-Gateway — REST + gRPC + Swagger + валидация

Единый сервис, доступный и по gRPC, и по HTTP/REST. REST API генерируется автоматически из proto-определений через gRPC-Gateway. Запросы валидируются через protovalidate, документация доступна в Swagger UI.

## Концепция

gRPC-Gateway — reverse-proxy, транслирующий HTTP/JSON-запросы в gRPC-вызовы и обратно. Маппинг HTTP-методов и путей на RPC описывается прямо в proto-файле через аннотации `google.api.http`:

```proto
rpc Create(CreateRequest) returns (CreateResponse) {
  option (google.api.http) = {
    post: "/api/v1/ufo"
    body: "*"
  };
}
```

Из того же proto генерируются:
- gRPC-сервер и клиент (как обычно)
- HTTP reverse-proxy (grpc-gateway)
- OpenAPI-спецификация (swagger.json)
- Валидационные правила (protovalidate)

Один proto-файл — единый источник правды для обоих протоколов.

## Архитектура

```
Клиент (curl/browser)               gRPC клиент
        |                                  |
        v                                  v
   HTTP :8081                        gRPC :50051
   (gRPC-Gateway)  ──── прокси ────>  gRPC Server
        |
   Swagger UI (/swagger-ui)
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| HTTP-аннотации в proto | `proto/ufo/v1/ufo.proto` | `google.api.http` — маппинг REST на RPC |
| Правила валидации | `proto/ufo/v1/ufo.proto` | `buf.validate` — min_len, max_len прямо в proto |
| Запуск двух серверов | `cmd/grpc_server/main.go` | gRPC на :50051, HTTP+Swagger на :8081 в одном процессе |
| Gateway-прокси | `cmd/grpc_server/main.go` | `runtime.NewServeMux` + регистрация handler'ов |
| Встроенный Swagger UI | `api/swagger-ui.html`, `static/embed.go` | Статика через `embed.FS` |
| Сгенерированная OpenAPI-спека | `api/swagger.json` | Генерируется из proto, не пишется руками |
| buf.gen.yaml | `proto/buf.gen.yaml` | Плагины: go, go-grpc, grpc-gateway, openapiv2, validate |

## Как запустить

```bash
task proto:gen   # генерация всего кода
task run         # gRPC :50051 + HTTP :8081
```

Swagger UI: http://localhost:8081

## Структура проекта

```
grpc_gateway_swagger_validation/
├── api/
│   ├── swagger.json       # сгенерированная OpenAPI-спека
│   └── swagger-ui.html    # Swagger UI
├── cmd/
│   ├── grpc_server/       # оба сервера (gRPC + HTTP)
│   └── grpc_client/       # клиент
├── proto/ufo/v1/          # proto с HTTP-аннотациями и валидацией
├── pkg/proto/             # сгенерированный код
├── static/embed.go        # встроенная статика
└── Taskfile.yaml
```
