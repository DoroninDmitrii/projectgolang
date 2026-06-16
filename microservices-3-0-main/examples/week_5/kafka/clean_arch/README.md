# Kafka в Clean Architecture — UFO Sightings

gRPC-сервис регистрации наблюдений НЛО с публикацией событий в Kafka и consumer для их обработки. Демонстрирует интеграцию Kafka в многослойную архитектуру с platform-абстракциями.

## Архитектура

```
gRPC Client
    │
    ▼
┌─────────────────┐
│  API (gRPC)     │  internal/api/ufo/v1/
│  Create/Get/... │
└────────┬────────┘
         │
    ┌────▼────┐
    │ Service │  internal/service/ufo/
    │         │  Create → repo.Create + producer.Produce
    └──┬───┬──┘
       │   │
  ┌────▼┐ ┌▼──────────┐
  │Repo │ │  Producer  │  internal/producer/ufo/
  │(PG) │ │  (Kafka)   │  proto.Marshal → platform.Send
  └─────┘ └─────┬──────┘
                │
         ┌──────▼──────┐
         │  Platform   │  platform/pkg/kafka/
         │  Kafka SDK  │  Consumer, Producer, Middleware
         └──────┬──────┘
                │
         ┌──────▼──────┐
         │   Sarama    │
         └─────────────┘

         ┌─────────────┐
         │  Consumer   │  internal/consumer/ufo/
         │  (Kafka)    │  decode protobuf → log
         └─────────────┘
```

## На что обратить внимание

### Platform-абстракции над Kafka

| Что | Файл | Зачем |
|-----|------|-------|
| Универсальный `Message` | `platform/pkg/kafka/message.go` | Отвязка от Sarama: `Key`, `Value`, `Headers` |
| `MessageHandler` — тип обработчика | `platform/pkg/kafka/kafka.go` | `func(ctx, Message) error` — единый контракт |
| `Middleware` — цепочка обработки | `platform/pkg/kafka/kafka.go` | `func(next Handler) Handler` — как HTTP middleware |
| Producer wrapper | `platform/pkg/kafka/producer/producer.go` | Привязка к одному topic, конвертация в Sarama |
| Consumer wrapper с rebalance-loop | `platform/pkg/kafka/consumer/consumer.go` | Бесконечный `Consume()` с обработкой ребалансировки |
| Group handler | `platform/pkg/kafka/consumer/group_handler.go` | `ConsumeClaim` → middleware chain → `MarkMessage` |
| Logging middleware | `platform/pkg/middleware/kafka/logging.go` | Пример middleware — логирование входящих сообщений |

### Интеграция Kafka в Clean Architecture

| Что | Файл | Зачем |
|-----|------|-------|
| Producer как зависимость сервиса | `ufo/internal/service/ufo/deps.go` | Интерфейс `UFOProducerService` наравне с `UFORepository` |
| Публикация в `Create` | `ufo/internal/service/ufo/create.go` | DB write + Kafka publish (dual write — не атомарно!) |
| Protobuf-сериализация события | `ufo/internal/producer/ufo/producer.go` | `proto.Marshal` → `kafka.Message` |
| Consumer с decode | `ufo/internal/consumer/ufo/handler.go` | Обработчик: decode protobuf → бизнес-логика |
| Event proto | `shared/proto/events/v1/ufo.proto` | Отдельные proto для событий (не gRPC) |

### DI и жизненный цикл

| Что | Файл | Зачем |
|-----|------|-------|
| Lazy DI container | `ufo/internal/app/di.go` | Nil-check инициализация, автоматический граф зависимостей |
| Closer (LIFO) | `platform/pkg/closer/closer.go` | Graceful shutdown: gRPC → consumer → producer → PG pool |
| App lifecycle | `ufo/internal/app/app.go` | Signal handling, параллельный запуск gRPC + Kafka consumer |
| Config через cleanenv | `ufo/internal/config/` | Отдельный файл на каждый компонент (grpc, pg, kafka, producer, consumer) |

**Dual write** — запись в БД и публикация в Kafka не атомарны. Если Kafka упадёт после коммита в БД, событие потеряется. В продакшне решается через Transactional Outbox.

## Как запустить

```bash
task up-all                              # PostgreSQL + Kafka + Kafka UI
task migrate:ufo:up                      # Миграции
go run ufo/cmd/grpc_server/main.go       # gRPC сервер + Kafka consumer
task down-all
```

## Структура проекта

```
clean_arch/
├── platform/
│   └── pkg/
│       ├── kafka/              # Абстракции: Message, Consumer, Producer, Middleware
│       ├── closer/             # LIFO graceful shutdown
│       ├── logger/             # slog init
│       └── middleware/kafka/   # Logging middleware
├── shared/
│   └── proto/
│       ├── ufo/v1/             # gRPC сервис
│       └── events/v1/          # Kafka события (protobuf)
├── ufo/
│   ├── cmd/grpc_server/        # Entry point
│   ├── internal/
│   │   ├── api/ufo/v1/         # gRPC handlers
│   │   ├── service/ufo/        # Бизнес-логика + publisher
│   │   ├── repository/ufo/     # PostgreSQL
│   │   ├── producer/ufo/       # Kafka producer (protobuf encoding)
│   │   ├── consumer/ufo/       # Kafka consumer (protobuf decoding)
│   │   ├── app/                # DI container + lifecycle
│   │   ├── config/             # Конфигурация по компонентам
│   │   ├── model/              # Domain + Events
│   │   ├── converter/          # Proto <-> Domain
│   │   └── errors/
│   └── migrations/
├── deploy/compose/             # Docker Compose (core + ufo)
└── ufo.env                     # Переменные окружения
```
