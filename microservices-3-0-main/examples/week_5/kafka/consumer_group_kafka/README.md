# Kafka — Consumer Groups

Распределённое чтение сообщений из Kafka с помощью Consumer Group: автоматическое распределение partitions между экземплярами consumer.

## Концепция

Consumer Group — механизм горизонтального масштабирования чтения. Kafka автоматически распределяет partitions между участниками группы:

```
3 partitions, 1 consumer  → consumer читает все 3
3 partitions, 3 consumers → каждый читает по 1
3 partitions, 5 consumers → 2 простаивают (partitions < consumers)
```

При добавлении/выбывании consumer происходит **rebalancing** — перераспределение partitions. На время ребалансировки все consumers группы приостанавливаются.

**Offset management** — группа хранит текущую позицию чтения в каждой partition. `MarkMessage()` фиксирует offset после обработки — at-least-once семантика.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| `RoundRobinPartitioner` в producer | `cmd/producer/main.go` | Равномерное распределение по partitions (дефолтный `HashPartitioner` отправляет все unkeyed в одну) |
| `ConsumerGroupHandler` — 3 метода | `cmd/consumer/main.go` | `Setup()` / `ConsumeClaim()` / `Cleanup()` — жизненный цикл при ребалансировке |
| Цикл `Consume()` в бесконечном loop | `cmd/consumer/main.go` | `Consume()` возвращается при ребалансировке — нужно вызвать снова |
| `session.MarkMessage()` | `cmd/consumer/main.go` | Коммит offset после обработки (не до!) |
| Явное создание topic с 3 partitions | `docker-compose.yml` | `kafka-init` сервис + `AUTO_CREATE_TOPICS_ENABLE=false` |

## Как запустить

```bash
task up                          # Kafka + init (создание topic) + UI
go run cmd/producer/main.go      # 20 сообщений в 3 partitions
go run cmd/consumer/main.go      # Запустить 1-3 экземпляра для наблюдения ребалансировки
task down
```

## Структура проекта

```
consumer_group_kafka/
├── cmd/
│   ├── producer/       # Producer с RoundRobin-партиционированием
│   └── consumer/       # Consumer group handler
├── docker-compose.yml  # Kafka + kafka-init (topic с 3 partitions) + UI
└── tests/
```
