# HTTP Chi — REST API

REST API для работы с данными о погоде на базе Chi-роутера. Позволяет сравнить подход HTTP/REST с gRPC из предыдущего примера.

## Концепция

Chi — легковесный HTTP-роутер, полностью совместимый со стандартным `net/http`. Поддерживает middleware-цепочки, группировку роутов и параметры URL.

Подробнее о таймаутах и защите HTTP-сервера — в [HTTP_SERVER.md](../HTTP_SERVER.md).

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Таймауты сервера | `cmd/http_server/main.go` | ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout — защита от Slowloris |
| Middleware-цепочка | `cmd/http_server/main.go` | Logger, Recoverer, Timeout, SetContentType — порядок важен |
| Группировка роутов | `cmd/http_server/main.go` | `r.Route("/api", ...)` — вложенные роуты с общим префиксом |
| Структурированное логирование | `cmd/http_server/main.go` | `log/slog` вместо `log.Printf` |

## Как запустить

```bash
task run        # сервер на :8080
task test:api   # API-тесты
```

## Структура проекта

```
http_chi/
├── cmd/
│   ├── http_server/   # сервер
│   └── http_client/   # клиент для тестирования
├── pkg/models/        # модель Weather
├── tests/             # API-тесты
└── Taskfile.yaml
```
