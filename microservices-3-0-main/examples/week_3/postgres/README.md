# PostgreSQL — raw SQL vs query builder

Два подхода к работе с PostgreSQL из Go: прямые SQL-запросы через pgx и программное построение запросов через Squirrel.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Одиночное соединение | `cmd/raw_query/main.go` | `pgx.Connect` — подходит для скриптов, не для сервисов |
| Пул соединений | `cmd/query_with_squirrel/main.go` | `pgxpool.New` — переиспользование соединений под нагрузкой |
| `sql.NullTime` | `cmd/raw_query/main.go` | Nullable поля в Go — `Valid` проверяет, есть ли значение |
| Dollar placeholders | `cmd/query_with_squirrel/main.go` | `sq.Dollar` — формат `$1, $2` специфичен для PostgreSQL (MySQL использует `?`) |
| INSERT RETURNING | `cmd/query_with_squirrel/main.go` | `Suffix("RETURNING id")` — получаем id созданной записи без дополнительного запроса |

### Raw SQL vs Squirrel

**Raw SQL** (`cmd/raw_query/`) — минимум абстракций: `pool.Exec()`, `pool.Query()`, ручной `Scan` в структуру. Просто, но SQL-строки фрагментируются при добавлении условий.

**Squirrel** (`cmd/query_with_squirrel/`) — builder собирает запрос программно: `.Where()`, `.Limit()`, `.Set()`. Удобно для динамических фильтров, но добавляет зависимость.

## Как запустить

```bash
# Поднять PostgreSQL
task docker:compose:up

# Применить миграции
task migrate:up

# Запустить примеры
go run cmd/raw_query/main.go
go run cmd/query_with_squirrel/main.go

# Остановить
task docker:compose:down
```

## Структура проекта

```
postgres/
├── cmd/
│   ├── raw_query/             # Прямые SQL-запросы через pgx
│   └── query_with_squirrel/   # SQL-конструктор Squirrel
├── migrations/                # Goose-миграция (таблица note)
├── docker-compose.yml
├── .env
└── Taskfile.yaml
```
