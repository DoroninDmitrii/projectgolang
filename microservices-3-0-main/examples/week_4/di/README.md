# Dependency Injection — ручной DI-контейнер с lazy-инициализацией

Пример организации зависимостей микросервиса через DI-контейнер без внешних фреймворков (Wire, Uber FX). Контейнер с lazy-геттерами автоматически разрешает цепочки зависимостей при первом обращении.

## Концепция

Когда сервис растёт, main.go превращается в простыню инициализации: pool, repo, svc, api, cache, producer, consumer... Порядок создания зависит от неочевидных связей, добавление нового компонента требует аккуратной вставки в нужное место.

DI-контейнер решает это: каждый компонент описан геттером, который при первом вызове создаёт объект и запоминает его. Зависимости разрешаются рекурсивно:

```go
func (d *diContainer) UFOService(ctx context.Context) ufov1API.UFOService {
    if d.ufoSvc == nil {
        d.ufoSvc = ufoService.New(d.UFORepository(ctx))  // автоматически создаст repo и pool
    }
    return d.ufoSvc
}
```

Вызов `UfoV1API(ctx)` создаст всю цепочку: PGPool -> Repository -> Service -> API.

Trade-off: бойлерплейт на каждый геттер, но полная compile-time безопасность и нулевая магия.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| DI-контейнер | `ufo/internal/app/di.go` | Lazy-геттеры с nil-check, хранение интерфейсов, fail-fast через `os.Exit` |
| App lifecycle | `ufo/internal/app/app.go` | `initDeps` — цепочка инициализаций, `Run` синхронно ждёт сигнал/ошибку сервера и зовёт `closer.CloseAll` с отдельным контекстом |
| Closer (LIFO) | `platform/pkg/closer/closer.go` | Глобальный менеджер ресурсов: `Add("name", fn)` при создании, `CloseAll(ctx)` при shutdown |
| Порядок закрытия | `platform/pkg/closer/closer.go` | LIFO: gRPC-сервер (перестать принимать) -> сервисы -> БД (последней) |
| Health Check | `platform/pkg/grpc/health/health.go` | Стандартный gRPC Health Check Protocol |
| Структурное логирование | `platform/pkg/logger/logger.go` | slog с JSON-хендлером, настраиваемый уровень |
| Интерфейсы в deps.go | `ufo/internal/*/deps.go` | Каждый слой определяет нужный ему интерфейс, а не импортирует из зависимости |
| Тесты closer'а | `platform/pkg/closer/closer_test.go` | LIFO-порядок, конкурентность, `sync.Once`, отмена контекста |

## Архитектура

```
┌─────────────────────────────────────────┐
│                 App                      │
│  ┌─────────────────────────────────┐    │
│  │          diContainer             │    │
│  │                                  │    │
│  │  UfoV1API ──→ UFOService ──→    │    │
│  │       UFORepository ──→ PGPool  │    │
│  └─────────────────────────────────┘    │
│                                          │
│  gRPC Server ──→ Closer (LIFO shutdown) │
└─────────────────────────────────────────┘
```

Отличия от примера `config`:
- main.go минимален — вся инициализация в `App` и `diContainer`
- Graceful shutdown через `closer` вместо ручного `GracefulStop()` + `time.After`
- Platform-модуль с переиспользуемыми пакетами (closer, logger, health)

## Как запустить

```bash
task up                  # PostgreSQL
task migrate:ufo:up      # Миграции
task run                 # gRPC-сервер
task test:api            # Интеграционные тесты
task down                # Остановка
```

## Структура проекта

```
di/
├── platform/pkg/                 # Переиспользуемые пакеты
│   ├── closer/                   # LIFO graceful shutdown
│   ├── logger/                   # slog JSON-логгер
│   └── grpc/health/              # gRPC Health Check
├── shared/proto/ufo/v1/          # Proto-определения
├── ufo/
│   ├── cmd/grpc_server/          # Минимальный main.go
│   ├── internal/
│   │   ├── app/                  # App + DI-контейнер
│   │   ├── config/               # Env-конфигурация
│   │   ├── api/ufo/v1/           # gRPC-обработчики
│   │   ├── service/ufo/          # Бизнес-логика
│   │   ├── repository/ufo/       # PostgreSQL-запросы
│   │   ├── model/                # Доменные модели
│   │   └── converter/            # Proto <-> модель
│   └── tests/                    # Интеграционные тесты
└── deploy/compose/               # Docker Compose
```
