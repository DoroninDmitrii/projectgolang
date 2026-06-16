# Transaction — атомарные операции с go-transaction-manager

Паттерн Transaction Manager на примере банковских переводов. Репозиторий не знает о транзакциях — за это отвечает менеджер через `context.Context`.

## Концепция

### Transaction Manager

Проблема: бизнес-логика в сервисном слое должна выполнять несколько операций атомарно, но репозиторий не должен знать про `BEGIN`/`COMMIT`/`ROLLBACK` — иначе он становится привязан к конкретному сценарию.

Решение — `go-transaction-manager` (avito-tech): менеджер кладёт `pgx.Tx` в `context`, а репозиторий через `CtxGetter` автоматически использует транзакцию, если она есть, или пул, если нет.

```
┌─────────┐         ┌────────────┐         ┌──────────────┐
│ Service  │──Do()──▶│ TxManager  │──BEGIN──▶│  PostgreSQL  │
│          │         │            │          │              │
│          │◀─ctx────│  кладёт    │          │              │
│          │  с Tx   │  pgx.Tx    │          │              │
│          │         │  в ctx     │          │              │
│          │         └────────────┘          │              │
│          │                                 │              │
│          │──Get()──▶┌────────────┐         │              │
│          │          │ Repository │         │              │
│          │          │            │         │              │
│          │          │ CtxGetter: │         │              │
│          │          │ «есть Tx   │──SQL───▶│  выполняет   │
│          │          │  в ctx?    │  через  │  в рамках    │
│          │          │  да → Tx»  │  pgx.Tx │  транзакции  │
│          │          └────────────┘         │              │
│          │                                 │              │
│  nil ──▶ │ TxManager ─── COMMIT ─────────▶│              │
│  err ──▶ │ TxManager ─── ROLLBACK ───────▶│              │
└──────────┘                                 └──────────────┘
```

1. Сервис вызывает `txManager.Do(ctx, fn)`.
2. TxManager берёт соединение из пула, выполняет `BEGIN`, кладёт `pgx.Tx` в новый `ctx`.
3. Репозиторий через `getter.DefaultTrOrDB(ctx, pool)` достаёт транзакцию из контекста — или пул, если транзакции нет.
4. `fn` вернула `nil` — COMMIT. `fn` вернула `error` — ROLLBACK.

### DefaultCtxGetter vs кастомный

`DefaultCtxGetter` хранит транзакцию по общему ключу — достаточно для одной БД. Кастомный `CtxGetter` (через `trmpgx.NewCtxGetter`) нужен, когда в одном контексте живут несколько независимых транзакций: например, основная БД + БД аудита. Разные ключи — разные транзакции, они не перетирают друг друга.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| `CtxGetter` в репозитории | `internal/repository/account/repository.go` | `DefaultTrOrDB` — один код работает и в транзакции, и без |
| `QueryRow` + sentinel error | `internal/repository/account/get.go` | `pgx.ErrNoRows` → `ErrAccountNotFound` |
| `CollectRows` + `ANY($1)` | `internal/repository/account/list.go` | Массовый запрос по слайсу UUID |
| `RowsAffected()` | `internal/repository/account/update_balance.go` | Проверка, что UPDATE затронул запись |
| `TxManager` интерфейс | `internal/service/account/deps.go` | Сервис зависит от интерфейса, не от реализации |
| Атомарный перевод | `internal/service/account/transfer.go` | `Do()` оборачивает дебет + кредит в одну транзакцию |

## Как запустить

```bash
task docker:compose:up     # Поднять PostgreSQL
task migrate:up            # Применить миграции
task run                   # Запустить демо
task test:api              # Интеграционные тесты
task docker:compose:down   # Остановить
```

## Ожидаемый вывод

```
INFO подключение к PostgreSQL установлено
INFO --- Начальные балансы ---
INFO счёт owner=Алиса balance=1000000 description="Основной счёт"
INFO счёт owner=Боб balance=500000 description=<нет>
INFO счёт owner=Карл balance=250000 description=Сберегательный
INFO --- Перевод 100₽ от Алисы к Бобу ---
INFO перевод выполнен успешно
INFO --- Балансы после перевода ---
INFO счёт owner=Алиса balance=990000
INFO счёт owner=Боб balance=510000
INFO --- Попытка перевода 99999999 копеек от Карла к Алисе ---
INFO перевод отклонён: недостаточно средств (транзакция откачена)
INFO --- Балансы после отклонённого перевода (без изменений) ---
INFO счёт owner=Алиса balance=990000
INFO счёт owner=Боб balance=510000
INFO счёт owner=Карл balance=250000
```

## Структура проекта

```
transaction/
├── cmd/
│   └── main.go                  # Демо: создание счетов и переводы
├── internal/
│   ├── model/                   # Доменная модель Account
│   ├── repository/account/      # Слой данных (pgx + CtxGetter)
│   └── service/account/         # Бизнес-логика (Transfer + TxManager)
├── migrations/                  # Goose-миграция (таблица accounts)
├── docker-compose.yml
└── Taskfile.yaml
```
