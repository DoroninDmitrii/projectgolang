# Domain-Driven Design — Rich Domain Model на примере PC Builder

Пример DDD-подхода: сборка ПК с проверкой совместимости компонентов, резервированием со склада и отменой. Бизнес-правила живут в доменных моделях, а не в сервисах.

## Концепция

В типичном «анемичном» подходе модели — просто структуры с полями, а вся логика в сервисном слое. Проблема: инварианты (reserved <= stock, сокет CPU == сокет материнки) размазаны по коду и легко нарушаются.

DDD предлагает Rich Domain Model — модели сами защищают свои инварианты:

```go
// Приватная структура — нельзя создать в обход конструктора
type component struct { ... }
type Component = component  // Публичный alias

func (c *component) Reserve() error {
    if c.Available() <= 0 {
        return ErrOutOfStock  // Инвариант: reserved <= stock
    }
    c.reserved++
    return nil
}
```

Ключевые DDD-паттерны в примере:

- **Aggregate** — `PCBuild` и `Component` с методами, защищающими инварианты
- **Value Object** — `ComponentProperties` (socket, ram_type, tdp) хранится в JSONB
- **Domain Service** — `CompatibilityChecker` проверяет совместимость между агрегатами
- **Application Service** — оркестрирует транзакцию, вызывает доменные методы, не содержит бизнес-логики

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Rich Domain Model | `app/internal/model/entity/component.go` | `Reserve()`, `Release()`, `Available()` — инварианты в агрегате |
| Приватная структура + alias | `app/internal/model/entity/pc_build.go` | `type PCBuild = pcBuild` — нельзя создать в обход `RestorePCBuild()` |
| Domain Service | `app/internal/service/domain/compatibility_checker.go` | Четыре правила: материнка обязательна, совместимость сокета, типа RAM, TDP |
| Транзакция в use case | `app/internal/service/application/pc_builder/create_build.go` | `TxManager.Do()` оборачивает get -> check -> reserve -> persist |
| Батч-обновление резерва | `app/internal/repository/component/update_reserved_batch.go` | Одним запросом обновляет reserved для всех компонентов |
| Конкурентный тест | `app/tests/api_test.go` (`TestCreateBuild_ConcurrentReservation`) | Горутины, stock=1, ровно 1 успех — проверка блокировок |
| Полный жизненный цикл | `app/tests/api_test.go` (`TestFullLifecycle`) | Создание → проверка резервов → отмена → проверка освобождения |
| Value Object в JSONB | `app/internal/model/valueobject/component_properties.go` | `ComponentProperties` → JSONB в PostgreSQL |
| Record vs Model | `app/internal/repository/record/` vs `app/internal/model/entity/` | DB-записи отдельно от доменных моделей |
| Доменные ошибки | `app/internal/errors/errors.go` | `ErrOutOfStock`, `ErrIncompatibleSocket`, `ErrBuildAlreadyCancelled` |
| DI-контейнер | `app/internal/app/di.go` | Lazy-инициализация зависимостей: pool, repos, services, handler |

## Архитектура

```
API (Handler)
 │
 ▼
Application Service (pc_builder)         ← оркестрация, транзакция
 │
 ├──→ CompatibilityChecker               ← Domain Service (правила совместимости)
 │
 ├──→ Component.Reserve() / Release()    ← Rich Domain Model (инварианты)
 │    PCBuild.Cancel()
 │
 ├──→ ComponentRepository                ← FOR UPDATE + UpdateReserved
 └──→ BuildRepository                    ← Create + UpdateStatus
```

Сценарий CreateBuild внутри транзакции:

```
BEGIN
  → componentRepo.List(uuids)             -- получить компоненты
  → CompatibilityChecker.Check()           -- domain validation
  → component.Reserve() для каждого        -- domain logic
  → UpdateReservedBatch()                  -- persist (батч-обновление)
  → buildRepo.Create()                     -- persist
  → buildRepo.AddComponents()             -- привязать компоненты
COMMIT
```

## Как запустить

```bash
task up                  # PostgreSQL
task migrate:up          # Таблицы + seed-данные (8 компонентов)
task run                 # HTTP-сервер (порт 8080)
```

В отдельном терминале — демо-клиент с 8 сценариями:

```bash
go run ./app/cmd/demo/...
```

Тесты и остановка:

```bash
task test:api            # Интеграционные тесты (включая конкурентный)
task down                # Остановка
```

Корневой каталог теперь хранит `go.work`, а основной Go-модуль лежит в `app/`.

## Структура проекта

```
ddd/
├── go.work                                # Workspace: app + platform
├── app.env                                # Переменные окружения (DB_URI и т.д.)
├── migrations/                            # Схема + seed-данные
├── deploy/compose/ddd/                    # Docker Compose для PostgreSQL
├── app/
│   ├── cmd/
│   │   ├── app/main.go                    # HTTP-сервер
│   │   └── demo/main.go                   # Демо-клиент: 8 сценариев через HTTP API
│   ├── tests/api_test.go                  # Интеграционные тесты
│   └── internal/
│       ├── app/                           # Bootstrap: app.go + di.go (DI-контейнер)
│       ├── config/                        # Конфигурация: PG, HTTP, Logger
│       ├── model/
│       │   ├── entity/                    # Rich Domain Models
│       │   │   ├── component.go           # Aggregate: Reserve(), Release()
│       │   │   └── pc_build.go            # Aggregate: Cancel()
│       │   └── valueobject/               # Value Objects
│       │       ├── component_properties.go  # Pointer Union → JSONB
│       │       ├── cpu_properties.go      # Socket, TDP
│       │       ├── gpu_properties.go      # RequiredTDP
│       │       ├── motherboard_properties.go  # Socket, RAMType
│       │       ├── ram_properties.go      # RAMType
│       │       ├── component_type.go      # Enum
│       │       └── build_status.go        # Enum
│       ├── service/
│       │   ├── domain/                    # Domain Service
│       │   │   └── compatibility_checker.go
│       │   └── application/pc_builder/    # Use Cases (оркестрация)
│       ├── repository/                    # Data Access
│       │   ├── component/                 # CRUD + batch update
│       │   ├── build/                     # CRUD + add components
│       │   ├── record/                    # DB-записи (отдельно от моделей)
│       │   └── converter/                 # Record <-> Entity
│       ├── api/
│       │   ├── pc_builder/                # HTTP-обработчики
│       │   ├── dto/                       # Request/Response DTO
│       │   └── httputil/                  # JSON-хелперы
│       └── errors/                        # Доменные ошибки
└── platform/                              # Отдельный workspace-модуль (logger, closer)
```
