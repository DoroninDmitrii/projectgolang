# Многослойная архитектура (Clean Architecture)

Сервис регистрации наблюдений НЛО, построенный по принципам чистой архитектуры. Каждый слой изолирован через интерфейсы и конвертеры, зависимости направлены внутрь — к доменным моделям.

## Архитектура

```
gRPC-запрос
    |
    v
API слой        internal/api/ufo/v1/      валидация, proto <-> model
    |
    v
Service слой    internal/service/ufo/      бизнес-логика, оркестрация
    |           |
    v           v
Repository      WeatherClient             внешние зависимости (интерфейсы)
    |           |
    v           v
In-Memory       Stub / gRPC client
(sync.RWMutex)
```

Каждая граница между слоями имеет свой пакет конвертеров:
- `internal/api/converter/` — DTO <-> domain (на API-слое)
- `internal/repository/ufo/converter/` — domain <-> repository record

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Интерфейсы зависимостей | `internal/api/ufo/v1/deps.go`, `internal/service/ufo/deps.go` | Каждый слой объявляет свои зависимости как интерфейсы — Dependency Inversion |
| Graceful Degradation | `internal/service/ufo/create.go` | Если сервис погоды недоступен, наблюдение всё равно создаётся |
| Credibility scoring | `internal/service/ufo/credibility.go` | Бизнес-логика в service слое: оценка достоверности по заполненности полей |
| Soft delete | `internal/repository/ufo/delete.go`, `get.go` | Мягкое удаление через `DeletedAt`, get проверяет флаг |
| Модели репозитория | `internal/repository/ufo/record/` | Отдельные модели хранения, изолированные от домена |
| Подмена реализации | `internal/client/stub/weather/`, `internal/client/grpc/weather/v1/` | Два клиента погоды — stub и реальный gRPC, подключаются через DI в `main.go` |
| Конвертеры DTO <-> model | `internal/api/converter/ufo.go` | Wrapper types (`wrapperspb`) -> Go-указатели через `samber/lo` |
| Thread safety | `internal/repository/ufo/repository.go` | `sync.RWMutex` для in-memory хранилища |

## Как запустить

```bash
task run             # Запуск gRPC-сервера на :50051
task test:api        # E2E тесты через bufconn
task lint            # Линтинг
```

## Структура проекта

```
├── cmd/grpc_server/         точка входа, DI
├── internal/
│   ├── api/ufo/v1/          gRPC хэндлеры
│   ├── service/ufo/         бизнес-логика
│   ├── repository/ufo/      in-memory хранилище
│   │   ├── record/          модели хранения
│   │   └── converter/       domain <-> record
│   ├── converter/           proto <-> domain
│   ├── model/               доменные модели
│   ├── errors/              доменные ошибки
│   └── client/              внешние клиенты
│       ├── stub/weather/    stub-реализация
│       └── grpc/weather/v1/ gRPC-реализация
├── proto/                   .proto определения
├── pkg/proto/               сгенерированный код
└── tests/                   E2E тесты (bufconn)
```
