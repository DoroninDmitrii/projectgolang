# PostgreSQL — JSONB

Хранение гетерогенных свойств сущностей в JSONB-колонке: разные типы продуктов (laptop, phone, monitor) с разным набором атрибутов в одной таблице.

## Концепция

Проблема: продукты разных типов имеют разные атрибуты (CPU у ноутбука, battery_mah у телефона, refresh_rate у монитора). Варианты:

- **Отдельные таблицы** — table-per-type, неудобно при общих запросах
- **Широкая таблица** — много nullable колонок, большинство пустых
- **JSONB** — гибкая колонка, хранит произвольный JSON с индексацией

JSONB хранится в бинарном формате (быстрый доступ к полям), поддерживает GIN-индексы для поиска по вложенным ключам.

В Go: `json.Marshal()` при записи, `json.Unmarshal()` при чтении. Struct с `omitempty` тегами — заполняются только релевантные поля для типа продукта.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| `ProductProperties` с `omitempty` | `internal/model/product.go` | Один struct на все типы, неиспользуемые поля не попадают в JSON |
| `json.Marshal` при INSERT | `internal/repository/product/create.go` | Сериализация struct → `[]byte` → JSONB |
| `json.Unmarshal` при SELECT | `internal/repository/product/get.go` | JSONB читается как `[]byte`, потом десериализуется |
| Ручная итерация в `List` | `internal/repository/product/list.go` | `pgx.CollectRows` не работает с JSONB (нужен кастомный scan) |
| Полная замена свойств | `internal/repository/product/update_properties.go` | `SET properties = $1` — replace, не merge |
| Seed с inline JSONB | `migrations/00002_seed_products.sql` | Пример JSON-литералов в SQL |

## Как запустить

```bash
task docker:compose:up   # PostgreSQL
task migrate:up
task run                 # Демо: list → create → get → update → get
task test:api            # Интеграционные тесты (testcontainers)
task docker:compose:down
```

## Структура проекта

```
jsonb/
├── cmd/main.go                           # Демо: CRUD с JSONB
├── internal/
│   ├── model/product.go                  # Product + ProductProperties (JSONB struct)
│   ├── repository/product/
│   │   ├── create.go                     # json.Marshal → INSERT
│   │   ├── get.go                        # SELECT → json.Unmarshal
│   │   ├── list.go                       # Ручной scan (без CollectRows)
│   │   └── update_properties.go          # Полная замена JSONB
│   ├── service/product/                  # Thin service (delegation)
│   └── errors/
├── migrations/                           # Таблица с JSONB + seed data
├── tests/api_test.go                     # testcontainers + JSONB round-trip
└── docker-compose.yml
```
