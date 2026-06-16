# Easy Redis — базовые операции с Redis

Простейший пример работы с Redis через библиотеку redigo: строковые операции (SET/GET) и хеш-таблицы (HSET/HGET/HGETALL).

## На что обратить внимание

| Что | Файл |
|-----|------|
| SET/GET для простых пар ключ-значение | `cmd/main.go:setAndGet()` |
| HSET/HGET для отдельных полей хеш-таблицы | `cmd/main.go:printMapFieldsByOne()` |
| `redis.StringMap()` — HGETALL сразу в `map[string]string` | `cmd/main.go:printMapFields()` |
| `redis.ScanStruct()` — HGETALL в Go-структуру по тегам `redis:"field"` | `cmd/main.go:printMapFieldsByStruct()` |

## Как запустить

```bash
task up           # Поднять Redis
go run ./cmd/     # Запустить демо
task down         # Остановить Redis
```

## Структура проекта

```
easy_redis/
├── cmd/main.go            # SET/GET, HSET/HGET, три способа чтения хеша
├── tests/cli_test.go      # Тесты с testcontainers
└── docker-compose.yml     # Redis
```
