# Kafka — Batch Consumer

Пакетная обработка сообщений из Kafka: накопление буфера и обработка одной операцией вместо поштучной.

## Концепция

Поштучная обработка N сообщений = N сетевых round-trip (к БД, API и т.д.). Batch-обработка = 1 round-trip на пачку. При overhead 1ms на операцию и batch из 50 сообщений — экономия 49ms.

Два триггера flush:

```
Буфер достиг batchSize (50)  → flush полной пачки
Прошёл flushInterval (2s)    → flush неполной пачки (не зависать на ожидании)
Partition закрыта             → flush остатка
```

Буферизация ведётся **per-partition** — каждая partition имеет свой буфер, потому что offset в Kafka привязан к partition.

Операция обработки batch должна быть **атомарной**: если batch из 50 сообщений не обработан целиком — offset не коммитится, вся пачка будет перечитана.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Batch consumer с двумя триггерами | `cmd/batch_consumer/handler.go` | `batchSize` + `flushInterval` + flush при закрытии partition |
| Обработка пачки | `cmd/batch_consumer/process.go` | Имитация batch-операции (в продакшне: `INSERT ... VALUES ($1), ($2), ...`) |
| Поштучный consumer для сравнения | `cmd/consumer/main.go` | Базовый подход — видна разница в архитектуре |
| Producer 500 сообщений | `cmd/producer/main.go` | Достаточный объём для наблюдения батчинга |

## Как запустить

```bash
task up                                    # Kafka + init + UI
go run cmd/producer/main.go                # 500 сообщений
go run cmd/consumer/main.go                # Поштучный consumer
go run cmd/batch_consumer/main.go          # Batch consumer
task down
```

## Структура проекта

```
batch_kafka/
├── cmd/
│   ├── producer/         # 500 сообщений в 3 partitions
│   ├── consumer/         # Поштучная обработка
│   └── batch_consumer/   # Пакетная обработка
│       ├── handler.go    # ConsumerGroupHandler с буфером
│       ├── process.go    # Обработка пачки
│       └── main.go
├── docker-compose.yml
└── tests/
```
