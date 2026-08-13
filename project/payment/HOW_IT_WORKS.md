# Payment Service — как всё работает от начала до конца

## 1. Общая картина

Payment — это gRPC-сервис, который проводит «оплату» заказа. Он **не хранит состояние** —
на каждый запрос генерирует новый `transaction_uuid` и отвечает успехом (либо возвращает
ошибку валидации). Никакой базы, никакого учёта прошлых платежей — вся логика "уже оплачен"
или "нет" лежит на стороне Order-сервиса.

Слушает на порту `:50052`. Его вызывает Order-сервис, когда пользователь делает
`POST /api/v1/orders/{uuid}/pay`.

```
  Клиент (curl)
       │
       │  HTTP POST /api/v1/orders/{uuid}/pay
       ▼
  Order (:8080)
       │
       │  gRPC PayOrder(order_uuid, payment_method)
       ▼
  Payment (:50052)   ← ТЫ ЗДЕСЬ
       │
       │  Валидирует запрос
       │  Генерирует transaction_uuid
       ▼
  Ответ: {transaction_uuid: "..."}
```

Чем отличается от Inventory:
- Inventory хранит данные (map с 7 деталями) — Payment ничего не хранит.
- Inventory может вернуть `NOT_FOUND` — Payment либо `INVALID_ARGUMENT`, либо успех.
- У Inventory два метода — у Payment один.

---

## 2. Путь от .proto до работающего сервиса

### Шаг 1: Контракт (payment.proto)

Файл: `shared/proto/payment/v1/payment.proto`

```proto
service PaymentService {
  rpc PayOrder(PayOrderRequest) returns (PayOrderResponse);
}

enum PaymentMethod {
  PAYMENT_METHOD_UNSPECIFIED    = 0;
  PAYMENT_METHOD_CARD           = 1;
  PAYMENT_METHOD_SBP            = 2;
  PAYMENT_METHOD_CREDIT_CARD    = 3;
  PAYMENT_METHOD_INVESTOR_MONEY = 4;
}

message PayOrderRequest {
  string order_uuid = 1;
  PaymentMethod payment_method = 2;
}

message PayOrderResponse {
  string transaction_uuid = 1;
}
```

Один метод — `PayOrder`. Принимает UUID заказа и способ оплаты, возвращает UUID транзакции.

### Шаг 2: Генерация Go-кода (task proto:gen)

Как и для Inventory, `buf generate` создал два файла в `shared/pkg/proto/payment/v1/`:

| Файл | Что внутри |
|------|-----------|
| `payment.pb.go` | Go-структуры `PayOrderRequest`, `PayOrderResponse`, enum `PaymentMethod` |
| `payment_grpc.pb.go` | `PaymentServiceServer` (интерфейс сервера), `PaymentServiceClient` (интерфейс клиента), `RegisterPaymentServiceServer` |

Ключевой интерфейс, который реализует `service.go`:
```go
type PaymentServiceServer interface {
    PayOrder(context.Context, *PayOrderRequest) (*PayOrderResponse, error)
}
```

### Шаг 3: Реализация сервера (service.go)

Файл: `payment/pkg/service/service.go`

```go
type server struct {
    paymentv1.UnimplementedPaymentServiceServer
}

func NewServer() *server {
    return &server{}
}
```

Обрати внимание — **пустая структура**, в отличие от Inventory (там был `parts map[uuid.UUID]Part`).
Payment — stateless-сервис: ему нечего хранить между вызовами.

#### PayOrder — логика метода

```
Запрос: {order_uuid: "...", payment_method: PAYMENT_METHOD_CARD}
    │
    ├─ order_uuid пустой?              → INVALID_ARGUMENT
    ├─ order_uuid невалидный формат?   → INVALID_ARGUMENT
    ├─ payment_method == UNSPECIFIED?  → INVALID_ARGUMENT
    └─ всё ок → сгенерировать transaction_uuid (uuid.New())
              → залогировать через slog
              → вернуть PayOrderResponse{TransactionUuid: "..."}
```

Три проверки идут последовательно — как только одна проваливается, метод сразу
возвращает ошибку (ранний выход, без вложенных if).

**`uuid.Parse()` vs `uuid.New()`** — важное отличие, которое видно в этом сервисе:
- `uuid.Parse(строка)` — проверяет и разбирает **существующую** строку в UUID. Используется
  для валидации `order_uuid`, который пришёл в запросе.
- `uuid.New()` — генерирует **новый случайный** UUID v4. Используется для создания
  `transaction_uuid`, которого раньше не существовало.

**Почему сервис "всегда успешен"** (кроме валидации) — по замыслу задания, оплата
на этой неделе не имитирует отказы банка/платёжной системы. Задача недели — научиться
связывать HTTP↔gRPC, а не имитировать реальный процессинг.

### Шаг 4: Запуск сервера (main.go)

Файл: `payment/cmd/main.go` — устроен идентично `inventory/cmd/main.go`:

```go
func main() {
    lis, _ := net.Listen("tcp", ":50052")             // слушаем порт 50052
    grpcServer := grpc.NewServer()
    paymentv1.RegisterPaymentServiceServer(grpcServer, paymentService.NewServer())
    reflection.Register(grpcServer)                    // для grpcurl без .proto
    grpcServer.Serve(lis)                               // запуск
}
```

Единственное отличие от Inventory — другой порт (`:50052` вместо `:50051`) и другая
регистрация (`RegisterPaymentServiceServer` вместо `RegisterInventoryServiceServer`).

---

## 3. Как запустить и проверить

### Запуск сервиса

```bash
cd project
go run ./payment/cmd
```

Увидишь в консоли:
```
INFO запуск PaymentService адрес=:50052
```

### Проверка через grpcurl (во втором терминале)

**Успешная оплата картой:**
```bash
grpcurl -plaintext -d '{
  "order_uuid": "550e8400-e29b-41d4-a716-446655440001",
  "payment_method": "PAYMENT_METHOD_CARD"
}' localhost:50052 payment.v1.PaymentService/PayOrder
```

Ответ:
```json
{
  "transactionUuid": "b2f8e6d0-...-случайный-uuid"
}
```

Запусти команду ещё раз — `transaction_uuid` будет **другим** каждый раз (сервис не хранит
состояние, значит и не может вернуть "тот же" результат).

**Пустой order_uuid → ошибка:**
```bash
grpcurl -plaintext -d '{"payment_method": "PAYMENT_METHOD_CARD"}' \
  localhost:50052 payment.v1.PaymentService/PayOrder
```
Ожидается `InvalidArgument`.

**Невалидный формат UUID:**
```bash
grpcurl -plaintext -d '{"order_uuid": "не-uuid", "payment_method": "PAYMENT_METHOD_CARD"}' \
  localhost:50052 payment.v1.PaymentService/PayOrder
```
Ожидается `InvalidArgument`.

**Не указан способ оплаты (UNSPECIFIED по умолчанию):**
```bash
grpcurl -plaintext -d '{"order_uuid": "550e8400-e29b-41d4-a716-446655440001"}' \
  localhost:50052 payment.v1.PaymentService/PayOrder
```
Ожидается `InvalidArgument` — поле `payment_method` не передано, значит оно `0` = `UNSPECIFIED`.

---

## 4. Как Payment взаимодействует с другими сервисами

Payment **никого не вызывает** — он терминальный узел в цепочке (в отличие от Order,
который вызывает и Inventory, и Payment). Единственный, кто обращается к Payment — это Order.

```
Order (пункт 9, ещё не реализован)
    │
    │  1. Пользователь: POST /api/v1/orders/{uuid}/pay {"payment_method": "CARD"}
    │
    │  2. Order находит заказ в своей map, проверяет статус == PENDING_PAYMENT
    │
    │  3. Order конвертирует HTTP payment_method → gRPC enum:
    │     "CARD" → PAYMENT_METHOD_CARD (короткое имя → с префиксом)
    │
    │  4. Order вызывает gRPC-клиент:
    │     paymentClient.PayOrder(ctx, &paymentv1.PayOrderRequest{
    │         OrderUuid:     order.UUID,
    │         PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
    │     })
    ▼
Payment.PayOrder(...)
    │
    │  5. Валидирует, генерирует transaction_uuid, возвращает
    ▼
Order получает transaction_uuid
    │
    │  6. Order сохраняет transaction_uuid и payment_method в заказе
    │     Order меняет статус заказа: PENDING_PAYMENT → PAID
    │
    │  7. Order отвечает клиенту HTTP 200 с обновлённым заказом
```

**Важный нюанс для будущей реализации Order (пункт 9):** gRPC-ошибки от Payment
(`InvalidArgument`) нужно будет замаппить в HTTP-статусы через `status.FromError()`:

```go
st, ok := status.FromError(err)
if ok {
    switch st.Code() {
    case codes.InvalidArgument: // → HTTP 400
    case codes.NotFound:        // → HTTP 404
    default:                    // → HTTP 500
    }
}
```

---

## 5. Структура файлов

```
payment/
├── cmd/
│   └── main.go                        ← точка входа, запуск gRPC-сервера на :50052
├── pkg/
│   └── service/
│       └── service.go                 ← бизнес-логика (PayOrder)
├── go.mod                             ← зависимости (+ github.com/google/uuid)
└── go.sum

shared/
├── proto/
│   └── payment/v1/
│       └── payment.proto              ← КОНТРАКТ (ты написал)
└── pkg/
    └── proto/
        └── payment/v1/
            ├── payment.pb.go          ← СГЕНЕРИРОВАНО (структуры, enum)
            └── payment_grpc.pb.go     ← СГЕНЕРИРОВАНО (интерфейсы)
```

---

## 6. Команды

| Команда | Что делает |
|---------|------------|
| `go run ./payment/cmd` | Запускает сервис на `:50052` |
| `grpcurl -plaintext -d '{...}' localhost:50052 payment.v1.PaymentService/PayOrder` | Вызывает метод |
| `cd payment && go get github.com/google/uuid` | Добавляет зависимость uuid |
| `task lint` | Проверяет код линтером |
| `task format` | Форматирует код |

---

## 7. Ключевые отличия от Inventory (для сравнения)

| | Inventory | Payment |
|--|-----------|---------|
| Хранит состояние | Да (`map[uuid.UUID]Part`) | Нет (пустая структура) |
| Методов | 2 (`GetPart`, `ListParts`) | 1 (`PayOrder`) |
| Может вернуть `NOT_FOUND` | Да | Нет |
| Результат детерминирован | Да (для одного UUID всегда один ответ) | Нет (`transaction_uuid` новый каждый раз) |
| Порт | `:50051` | `:50052` |
