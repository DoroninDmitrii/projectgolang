# PostgreSQL — SELECT FOR UPDATE

Безопасный перевод средств между счетами: демонстрация проблемы lost update и решения через пессимистичную блокировку.

## Концепция

PostgreSQL по умолчанию работает в Read Committed. Две транзакции могут прочитать одно и то же значение баланса, независимо вычислить новое и перезаписать друг друга:

```
TX1: SELECT balance FROM accounts WHERE uuid='A'   → 1000
TX2: SELECT balance FROM accounts WHERE uuid='A'   → 1000  (не заблокировано!)
TX1: UPDATE SET balance = 900  (1000 - 100)
TX1: COMMIT
TX2: UPDATE SET balance = 900  (1000 - 100, но реально уже 900)
TX2: COMMIT
→ Потерян один перевод (lost update)
```

**SELECT FOR UPDATE** — пессимистичная row-level блокировка. Первая транзакция захватывает строку, вторая ждёт на SELECT до завершения первой.

### Предотвращение deadlock

При двусторонних переводах (A→B и B→A) возможен deadlock:

```
TX1 (A→B): блокирует A, ждёт B
TX2 (B→A): блокирует B, ждёт A  → DEADLOCK
```

Решение — **consistent lock ordering**: всегда блокировать в порядке сортировки UUID. Тогда обе транзакции сначала пытаются заблокировать меньший UUID.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| `TransferUnsafe` — lost update | `internal/service/account/transfer_unsafe.go` | Демонстрация проблемы: `Get()` без блокировки в транзакции |
| `Transfer` — безопасная версия | `internal/service/account/transfer.go` | `GetForUpdate()` + сортировка UUID для consistent lock ordering |
| `SELECT ... FOR UPDATE` | `internal/repository/account/get_for_update.go` | Row-level exclusive lock до конца транзакции |
| Concurrent transfer тест | `tests/api_test.go` | 50 горутин, проверка сохранения суммы балансов |
| Bidirectional deadlock тест | `tests/api_test.go` | 20 горутин A→B и B→A одновременно |
| `go-transaction-manager` | `internal/service/account/deps.go` | Управление транзакциями через контекст |

## Как запустить

```bash
task docker:compose:up   # PostgreSQL
task migrate:up
task run                 # Демо: 20 горутин unsafe vs safe transfer
task test:api            # Интеграционные тесты (testcontainers)
task docker:compose:down
```

Демо выводит ожидаемые и фактические балансы — при unsafe-переводе они не совпадают, при safe — совпадают.

## Структура проекта

```
select_for_update/
├── cmd/main.go                          # Демо: unsafe vs safe transfer
├── internal/
│   ├── model/account.go                 # Баланс в копейках (int64)
│   ├── repository/account/
│   │   ├── get.go                       # Обычный SELECT
│   │   ├── get_for_update.go            # SELECT ... FOR UPDATE
│   │   └── update_balance.go
│   ├── service/account/
│   │   ├── transfer_unsafe.go           # Lost update demo
│   │   └── transfer.go                  # Safe: lock ordering + FOR UPDATE
│   └── errors/
├── migrations/                          # Таблица + seed (Alice, Bob, Karl)
├── tests/api_test.go                    # testcontainers + concurrent tests
└── docker-compose.yml
```
