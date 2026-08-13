# Как работает вся система целиком

Общая карта проекта: три микросервиса, два протокола, два генератора кода.
Разборы отдельных сервисов — в `inventory/HOW_IT_WORKS.md`, `payment/HOW_IT_WORKS.md`,
`order/HOW_IT_WORKS.md`. Здесь — как всё это соединяется.

---

## 1. Схема системы

```
                    ┌─────────────────────────────┐
   Клиент           │        Order (:8080)        │
   (curl,  ────────►│  HTTP + JSON снаружи        │
   Postman)  HTTP   │  gRPC-клиент внутрь         │
                    │  хранит заказы в памяти     │
                    └──────────┬──────────────────┘
                               │
                   ┌───────────┴───────────┐
             gRPC  │                       │  gRPC
                   ▼                       ▼
        ┌────────────────────┐  ┌────────────────────┐
        │ Inventory (:50051) │  │  Payment (:50052)  │
        │ каталог деталей    │  │  оплата заказов    │
        │ 7 деталей в памяти │  │  stateless         │
        └────────────────────┘  └────────────────────┘
```

| Сервис | Порт | Протокол | Роль | Состояние |
|--------|------|----------|------|-----------|
| **Order** | 8080 | HTTP | принимает пользователя, координирует остальных | заказы в `map` |
| **Inventory** | 50051 | gRPC | каталог деталей: наличие, цена, остаток | 7 деталей в `map` |
| **Payment** | 50052 | gRPC | «проводит» оплату, выдаёт `transaction_uuid` | нет |

Почему так: **HTTP+JSON — наружу** (человекочитаемо, работает из любого браузера
и curl), **gRPC — внутрь** (бинарный, быстрый, строго типизированный, контракт
проверяется компилятором).

---

## 2. С чего всё начинается: контракты

Порядок работы над проектом был именно такой — сначала контракты, потом код:

```
ШАГ 1. Описать контракты (руками)

  shared/proto/inventory/v1/inventory.proto     ← gRPC API каталога
  shared/proto/payment/v1/payment.proto         ← gRPC API оплаты
  shared/api/order/v1/order.openapi.yaml        ← HTTP API заказов (был готов)

ШАГ 2. Сгенерировать код (командами)

  task proto:gen  →  buf  →  shared/pkg/proto/…      (структуры + клиент/сервер gRPC)
  task ogen:gen   →  ogen →  shared/pkg/openapi/…    (HTTP-роутер, DTO, интерфейс Handler)

ШАГ 3. Написать бизнес-логику (руками)

  inventory/pkg/service/service.go    реализует InventoryServiceServer
  payment/pkg/service/service.go      реализует PaymentServiceServer
  order/pkg/handler/handler.go        реализует orderv1.Handler

ШАГ 4. Написать инфраструктуру (руками)

  inventory/cmd/main.go   gRPC-сервер + keepalive + graceful shutdown
  payment/cmd/main.go     то же самое
  order/cmd/main.go       gRPC-клиенты + HTTP-сервер + таймауты + graceful shutdown
```

**Главная идея:** контракт — источник истины. Из одного `.proto` генерируется
и серверный интерфейс (для Inventory), и клиентский (для Order). Обе стороны
физически не могут разойтись — они порождены одним файлом.

---

## 3. Полный путь запроса: создание заказа

Что происходит от `curl` до ответа, по шагам через все файлы:

```
┌─ КЛИЕНТ ────────────────────────────────────────────────────────────────┐
│ curl -X POST localhost:8080/api/v1/orders                               │
│      -d '{"hull_uuid":"…0001","engine_uuid":"…0003"}'                   │
└────────────────────────────┬────────────────────────────────────────────┘
                             │ HTTP/1.1, JSON
                             ▼
┌─ ORDER: сгенерированный код (shared/pkg/openapi/order/v1/) ─────────────┐
│ • роутер: POST /api/v1/orders → операция createOrder                    │
│ • проверка Content-Type, разбор JSON                                    │
│ • обязательные поля на месте? формат UUID верный?  нет → 400 клиенту    │
│ • собирает CreateOrderRequest{HullUUID, EngineUUID, …}                  │
└────────────────────────────┬────────────────────────────────────────────┘
                             │ вызов Go-метода
                             ▼
┌─ ORDER: твой код (order/pkg/handler/handler.go → CreateOrder) ──────────┐
│ • собирает список UUID тех деталей, что пришли                          │
│ • вызывает h.inventoryClient.ListParts(ctx, …)                          │
└────────────────────────────┬────────────────────────────────────────────┘
                             │
                             ▼
┌─ ORDER: сгенерированный gRPC-клиент (shared/pkg/proto/inventory/v1/) ───┐
│ • сериализует ListPartsRequest в бинарный protobuf                      │
│ • отправляет по HTTP/2 на localhost:50051                               │
└────────────────────────────┬────────────────────────────────────────────┘
                             │ сеть (TCP)
                             ▼
┌─ INVENTORY: gRPC-фреймворк ─────────────────────────────────────────────┐
│ • принимает соединение, определяет метод ListParts                      │
│ • десериализует байты в *ListPartsRequest                               │
└────────────────────────────┬────────────────────────────────────────────┘
                             │
                             ▼
┌─ INVENTORY: твой код (inventory/pkg/service/service.go → ListParts) ────┐
│ • uuids непустой → режим «поиск по конкретным UUID»                     │
│ • для каждого: uuid.Parse → невалидный?  → status INVALID_ARGUMENT      │
│                map lookup  → не найден?  → status NOT_FOUND             │
│ • конвертирует внутренний Part → protobuf inventoryv1.Part              │
│ • возвращает ListPartsResponse{Parts: […]} в порядке запроса            │
└────────────────────────────┬────────────────────────────────────────────┘
                             │ ответ тем же путём обратно
                             ▼
┌─ ORDER: твой код, продолжение ──────────────────────────────────────────┐
│ • err != nil? → status.FromError → NotFound→404, InvalidArgument→400    │
│ • для каждой детали: StockQuantity <= 0 → 409                           │
│                      иначе totalPrice += Price                          │
│ • собирает Order{OrderUUID: uuid.New(), …, Status: PENDING_PAYMENT}     │
│ • store.mu.Lock() → orders[uuid] = order → Unlock()                     │
│ • возвращает &orderv1.CreateOrderResponse{OrderUUID, TotalPrice}        │
└────────────────────────────┬────────────────────────────────────────────┘
                             │
                             ▼
┌─ ORDER: сгенерированный код ────────────────────────────────────────────┐
│ • смотрит на ТИП возвращённого значения → CreateOrderResponse → 201     │
│ • сериализует в JSON                                                    │
└────────────────────────────┬────────────────────────────────────────────┘
                             ▼
┌─ КЛИЕНТ ────────────────────────────────────────────────────────────────┐
│ HTTP/1.1 201 Created                                                    │
│ {"order_uuid":"071ef6a8-…","total_price":800000}                        │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Полный путь запроса: оплата

Отличие — второй gRPC-сервис и изменение состояния:

```
curl -X POST localhost:8080/api/v1/orders/{uuid}/pay -d '{"payment_method":"CARD"}'
    │
    ▼
ogen: разбирает путь ({order_uuid}) И тело ({payment_method})
      проверяет, что "CARD" входит в enum        не входит → 400
    │
    ▼
handler.PayOrder:
    store.mu.Lock()                              ← один замок на всю операцию
    заказ есть?              нет → 404
    статус PENDING_PAYMENT?  нет → 409           (уже оплачен или отменён)
    "CARD" → PAYMENT_METHOD_CARD                 ← маппинг двух разных enum
    │
    ▼
paymentClient.PayOrder → сеть → Payment (:50052)
    │
    ▼
payment/service.go PayOrder:
    order_uuid пустой/кривой?      → INVALID_ARGUMENT
    payment_method UNSPECIFIED?    → INVALID_ARGUMENT
    иначе: transaction := uuid.New(), лог, возврат
    │
    ▼ (обратно в Order, замок всё ещё держится)
handler.PayOrder:
    uuid.Parse(transaction_uuid)                 ← строка → uuid.UUID
    order.TransactionUUID = &transaction
    order.PaymentMethod   = &pm
    order.Status          = PAID
    orders[uuid] = order                         ← обязательно записать копию обратно!
    store.mu.Unlock()                            ← через defer
    │
    ▼
HTTP 200 {"transaction_uuid":"…"}
```

**Почему `Lock`, а не `RLock`:** между «проверили статус» и «записали новый» есть окно.
При раздельных блокировках два параллельных запроса оба прошли бы проверку — и заказ
оплатился бы дважды. Один `Lock` на всю операцию закрывает эту гонку.

---

## 5. Как ошибки поднимаются снизу вверх

Одна ошибка проходит три преобразования:

```
INVENTORY                          ORDER                        КЛИЕНТ
─────────                          ─────                        ──────

status.Errorf(                     status.FromError(err)        HTTP/1.1 404
  codes.NotFound,        ────►     st.Code()==NotFound   ────►  {"code":404,
  "деталь не найдена")             │                             "message":
                                   ▼                              "деталь не найдена"}
     gRPC-код                 &CreateOrderNotFound{...}
                                        │
                                        ▼
                              ogen: тип → HTTP-статус
```

Таблица соответствия (гайд, раздел 5.3):

| gRPC-код | HTTP | Когда |
|----------|------|-------|
| `NotFound` | 404 | детали нет в каталоге / заказа нет в хранилище |
| `InvalidArgument` | 400 | кривой UUID, не указан способ оплаты |
| прочее | 500 | непредвиденная ошибка |

Плюс два случая, где ошибку порождает сам Order без участия gRPC:
- **409** — конфликт статусов (`StockQuantity <= 0`, повторная оплата, отмена оплаченного)
- **400** — выдаёт ogen ещё до хендлера (кривой JSON, отсутствующее поле, чужой enum)

---

## 6. Где какие данные живут

```
INVENTORY                    PAYMENT                   ORDER
─────────                    ───────                   ─────
map[uuid.UUID]Part           (ничего)                  map[uuid.UUID]Order
7 деталей из seed                                      создаётся при POST /orders
только чтение                                          RWMutex, чтение+запись
не меняется                                            пропадает при перезапуске
```

Важные следствия:
- **`stock_quantity` не уменьшается** при заказе — Inventory только читает свою map.
  Проверка `> 0` есть, списания нет (это правило недели: «не делать резервирование»).
- **Payment ничего не помнит** — на один и тот же `order_uuid` вернёт разные
  `transaction_uuid`. Логику «уже оплачен» держит Order, проверяя статус заказа.
- **Всё в памяти** — перезапуск любого сервиса стирает его данные. Поэтому `order_uuid`
  из прошлого запуска после рестарта даст 404.

---

## 7. Карта файлов: что руками, что генерируется

```
project/
│
├── shared/                          ← контракты и сгенерированный код
│   ├── proto/                       ✍️  РУКАМИ: gRPC-контракты
│   │   ├── inventory/v1/inventory.proto
│   │   ├── payment/v1/payment.proto
│   │   ├── buf.yaml                 конфиг линтера proto
│   │   └── buf.gen.yaml             конфиг генерации (какие плагины, куда класть)
│   ├── api/order/v1/                ✍️  HTTP-контракт (был готов в заготовке)
│   │   └── order.openapi.yaml
│   └── pkg/                         🤖 СГЕНЕРИРОВАНО, не редактировать
│       ├── proto/inventory/v1/          buf: структуры + клиент/сервер
│       ├── proto/payment/v1/            buf: то же
│       └── openapi/order/v1/            ogen: HTTP-роутер, DTO, Handler
│
├── inventory/
│   ├── pkg/service/service.go       ✍️  GetPart, ListParts + seed-данные
│   └── cmd/main.go                  ✍️  gRPC-сервер, keepalive, graceful shutdown
│
├── payment/
│   ├── pkg/service/service.go       ✍️  PayOrder
│   └── cmd/main.go                  ✍️  то же, что у inventory (порт :50052)
│
├── order/
│   ├── pkg/handler/handler.go       ✍️  Create/Get/Pay/Cancel + Order, orderStore
│   ├── cmd/main.go                  ✍️  gRPC-клиенты, HTTP-сервер, shutdown
│   └── tests/api_test.go            📋 эталонные тесты (не менять)
│
├── go.work                          объединяет 4 модуля в одно пространство
├── Taskfile.yaml                    команды проекта
├── .golangci.yml                    правила линтера
└── bin/                             инструменты (buf, ogen, линтеры)
```

**Правило:** если файл лежит в `shared/pkg/` — он сгенерирован. Правки в нём затрутся
при следующем `task proto:gen` / `task ogen:gen` (в `buf.gen.yaml` стоит `clean: true`).
Чтобы изменить API — правь контракт и перегенерируй.

---

## 8. Три пары «контракт → генератор → код»

```
inventory.proto ──buf──► InventoryServiceServer  ← реализует inventory/service.go
                └──────► InventoryServiceClient  ← вызывает   order/handler.go

payment.proto ────buf──► PaymentServiceServer    ← реализует payment/service.go
                └──────► PaymentServiceClient    ← вызывает   order/handler.go

order.openapi.yaml ─ogen► Handler (интерфейс)    ← реализует order/handler.go
                   └────► Server (HTTP-роутер)   ← запускает  order/main.go
```

Из одного контракта рождаются **обе стороны** взаимодействия. Именно поэтому
контракт правят в одном месте, а меняется поведение сразу и у клиента, и у сервера.

---

## 9. Запуск

```bash
cd project

# три терминала
go run ./inventory/cmd    # :50051
go run ./payment/cmd      # :50052
go run ./order/cmd        # :8080   (требует, чтобы первые два уже работали)
```

Порядок важен: Order при старте открывает gRPC-соединения к обоим сервисам.
(Строго говоря, `grpc.NewClient` ленивый — соединение устанавливается при первом
вызове, поэтому Order стартует и без них, но первый же запрос упадёт с 500.)

Проверка gRPC-сервисов напрямую, без Order:

```bash
grpcurl -plaintext -d '{"uuid":"550e8400-e29b-41d4-a716-446655440001"}' \
  localhost:50051 inventory.v1.InventoryService/GetPart

grpcurl -plaintext -d '{"order_uuid":"550e8400-e29b-41d4-a716-446655440001",
                        "payment_method":"PAYMENT_METHOD_CARD"}' \
  localhost:50052 payment.v1.PaymentService/PayOrder
```

Работает благодаря `reflection.Register(grpcServer)` в обоих `main.go` — grpcurl
спрашивает у сервиса его схему, не требуя `.proto`-файлов.

---

## 10. Команды

| Команда | Что делает | Когда |
|---------|------------|-------|
| `task setup` | ставит buf, ogen, линтеры в `./bin` | один раз при старте |
| `task proto:gen` | `.proto` → Go (buf) | после правки `.proto` |
| `task ogen:gen` | OpenAPI → Go (ogen) | после правки `.yaml` |
| `task format` | gofumpt + gci | перед коммитом |
| `task lint` | golangci-lint | перед коммитом |
| `task test:api` | эталонные тесты | перед коммитом |
| `task run:inventory` / `run:payment` / `run:order` | запуск сервиса | разработка |

Тесты поднимают все три сервиса **в одном процессе**: gRPC через `bufconn`
(соединение в памяти, без реальных портов), HTTP через `httptest`. Поэтому
для `task test:api` не нужно ничего запускать заранее.

---

## 11. Ограничения этой недели

Сознательные упрощения, чтобы сфокусироваться на связке HTTP↔gRPC:

| Не делаем | Почему |
|-----------|--------|
| слои `api/service/repository` | архитектура — неделя 2 |
| dependency injection | клиенты создаются прямо в `main` |
| база данных | всё в `map` в памяти |
| метрики, трейсы | неделя 7 |
| конфиги, env | порты захардкожены |
| резервирование деталей | только проверка `stock_quantity > 0` |
| идемпотентность, авторизация | неделя 6 |
| обёртка ошибок `fmt.Errorf("%w")` | неделя 2 |

Что делаем обязательно: логи через `slog`, gRPC-ошибки через `status.Error`,
маппинг gRPC-кодов в HTTP через `status.FromError`, весь пользовательский текст
на русском.
