# Kafka — базовые Producer и Consumer

Минимальный пример работы с Apache Kafka через библиотеку Sarama: отправка и чтение сообщений.

## Концепция

Kafka — распределённый лог сообщений. Producer записывает сообщения в topic, Consumer читает их. Topic разбит на partitions — параллельные потоки записи/чтения.

Два режима отправки:

- **Sync Producer** — `SendMessage()` блокирует до получения ACK от брокера. Throughput = N * latency, зато fail-fast: если сообщение #3 не доставлено, #4 не отправляется.
- **Async Producer** — сообщения буферизуются в канал `Input()` и отправляются пайплайном. Throughput ~ 1 * latency (все сообщения in-flight одновременно). Обязательно читать из `Successes()` и `Errors()`, иначе deadlock.

Простой Consumer (без consumer group) — ручное указание partition, без коммита offset. Годится для отладки.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Sync producer с `WaitForAll` | `cmd/producer/main.go` | Гарантия записи на все реплики |
| Async producer с каналами | `cmd/async_producer/main.go` | Обязательное чтение `Successes()`/`Errors()` |
| Бенчмарк sync vs async | `cmd/compare_producers/main.go` | ~20x разница на 20 сообщениях |
| Consumer без группы | `cmd/consumer/main.go` | Ручной выбор partition, `OffsetOldest` |

## Как запустить

```bash
task up                              # Kafka + Kafka UI (порт 8090)
go run cmd/producer/main.go          # Sync producer
go run cmd/async_producer/main.go    # Async producer
go run cmd/compare_producers/main.go # Бенчмарк
go run cmd/consumer/main.go          # Consumer
task down
```

## Структура проекта

```
easy_kafka/
├── cmd/
│   ├── producer/           # Sync producer
│   ├── async_producer/     # Async producer
│   ├── compare_producers/  # Бенчмарк sync vs async
│   └── consumer/           # Простой consumer (без группы)
├── docker-compose.yml      # Kafka (KRaft, без Zookeeper) + UI
└── tests/
```
