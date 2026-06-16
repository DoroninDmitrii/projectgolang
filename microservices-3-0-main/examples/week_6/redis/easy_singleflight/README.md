# Singleflight — защита от cache stampede

Пакет `golang.org/x/sync/singleflight` группирует одинаковые одновременные запросы: если 50 горутин запрашивают одни и те же данные, к БД уходит только 1 запрос.

## Концепция

**Cache stampede** (thundering herd) — TTL кеша истёк, N горутин одновременно видят промах и все идут в БД за одними и теми же данными. При высоком трафике (Black Friday, вирусный пост) это может уронить базу.

`sf.Do(key, fn)` гарантирует: первый вызов с данным ключом выполняет `fn`, остальные блокируются и получают тот же результат. Работает in-process (на одной машине). Для распределённой защиты — использовать distributed lock.

## Демо

Программа последовательно показывает два сценария:

1. **Без singleflight** — 50 горутин, 50 запросов к БД
2. **С singleflight** — 50 горутин, 1 запрос к БД

## На что обратить внимание

| Что | Файл |
|-----|------|
| `GetProduct` без защиты vs `GetProductSF` с singleflight | `internal/cache/get_product.go`, `internal/cache/get_product_sf.go` |
| Redis pipeline: HSET + EXPIRE в одном round-trip | `internal/cache/cache.go:Set()` |
| Ошибка записи в кеш логируется, но не влияет на результат | `internal/cache/get_product_sf.go` |

## Как запустить

```bash
task up          # Поднять Redis
go run ./cmd/    # Запустить демо
task down        # Остановить Redis
```

## Структура проекта

```
easy_singleflight/
├── cmd/
│   ├── main.go       # Точка входа, подключение к Redis
│   └── demo.go       # Демо-сценарии (stampede и singleflight)
├── internal/cache/
│   └── cache.go      # Cache struct, Set/Get, GetProduct/GetProductSF
└── tests/            # Тесты (testcontainers + Redis)
```
