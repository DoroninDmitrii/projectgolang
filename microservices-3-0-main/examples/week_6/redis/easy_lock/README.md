# Distributed Lock — распределённые блокировки на Redis

Распределённая блокировка (mutex) для защиты от конкурентного выполнения и дублирования операций между несколькими процессами.

## Концепция

Типичная ситуация: Kafka отправляет одно сообщение нескольким воркерам (fan-out), и все обрабатывают один заказ — баланс списывается многократно. Нужен механизм, который гарантирует:
1. **Mutual exclusion** — только один процесс работает с ресурсом одновременно.
2. **Exactly-once** — операция выполняется ровно один раз, даже при повторных попытках.
3. **Deadlock protection** — если процесс упал, блокировка автоматически снимется через TTL.

Решение: `SET key UUID NX EX ttl` для захвата, `DELEX key IFEQ UUID` (Redis 8.0+) для освобождения только владельцем.

## Демо-сценарии

Программа последовательно показывает четыре ситуации:

| Сценарий | Что происходит |
|----------|----------------|
| **Problem** | 5 воркеров без блокировки → баланс списан 5 раз |
| **Solution** | Lock + Idempotency Key (OnceExecutor) → операция выполнена ровно 1 раз |
| **Competition** | Воркеры с retry ждут освобождения лока, обрабатывают по очереди |
| **TTL Protection** | Воркер «крашится» с локом → TTL автоматически снимает блокировку |

## На что обратить внимание

| Что | Файл |
|-----|------|
| Захват: `SET key UUID NX EX ttl`, освобождение: `DELEX key IFEQ UUID` | `internal/lock/lock.go` |
| UUID как identity владельца — чужой процесс не может снять чужой лок | `internal/lock/lock.go:Release()` |
| `AcquireWithRetry` через `time.NewTimer` + `select` для корректной отмены контекстом | `internal/lock/lock.go:AcquireWithRetry()` |
| OnceExecutor: double-checked locking (fast path без лока + повторная проверка после захвата) | `internal/lock/once.go:Do()` |
| Idempotency key с timestamp — видно в Redis, когда операция была выполнена | `internal/lock/once.go:Do()` |
| При ошибке бизнес-логики лок снимается, но idempotency key не ставится → retry возможен | `internal/lock/once.go:Do()` |

## Как запустить

```bash
task up           # Поднять Redis
go run ./cmd/     # Запустить все 4 демо
task down         # Остановить Redis
```

## Структура проекта

```
easy_lock/
├── cmd/
│   ├── main.go              # Точка входа, подключение к Redis
│   ├── demo_problem.go      # Проблема: дублирование без блокировки
│   ├── demo_solution.go     # Решение: OnceExecutor (lock + idempotency key)
│   ├── demo_competition.go  # Конкуренция: retry с ожиданием
│   └── demo_ttl.go          # TTL: защита от deadlock при крэше
├── internal/lock/
│   ├── lock.go              # Distributed Lock (Acquire/Release/AcquireWithRetry)
│   └── once.go              # OnceExecutor (distributed sync.Once)
└── tests/                   # Тесты с testcontainers
```
