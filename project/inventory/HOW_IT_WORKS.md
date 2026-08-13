# Inventory Service — как всё работает от начала до конца

## 1. Общая картина

Inventory — это gRPC-сервис, который хранит каталог деталей для космических кораблей
и отвечает на два вопроса:
- **GetPart** — «дай деталь по UUID»
- **ListParts** — «дай список деталей (по UUID или по типу)»

Он слушает на порту `:50051`. Его вызывает Order-сервис (HTTP :8080), когда пользователь
создаёт заказ — Order спрашивает у Inventory, существуют ли детали и есть ли они на складе.

```
  Клиент (curl)
       │
       │  HTTP POST /api/v1/orders
       ▼
  Order (:8080)
       │
       │  gRPC ListParts(uuids: [...])
       ▼
  Inventory (:50051)   ← ТЫ ЗДЕСЬ
       │
       │  Ищет в map[uuid.UUID]Part
       ▼
  Ответ: список деталей с ценами и наличием
```

---

## 2. Путь от .proto до работающего сервиса

### Шаг 1: Описание контракта (.proto)

Всё начинается с файла `shared/proto/inventory/v1/inventory.proto`.
Это **контракт** — описание API на языке Protocol Buffers:

```proto
service InventoryService {
  rpc GetPart(GetPartRequest) returns (GetPartResponse);
  rpc ListParts(ListPartsRequest) returns (ListPartsResponse);
}

message GetPartRequest {
  string uuid = 1;
}

message GetPartResponse {
  Part part = 1;
}
```

Здесь описано:
- Какие **методы** есть у сервиса (`GetPart`, `ListParts`)
- Какие **данные** принимает и возвращает каждый метод (Request/Response)
- Какие **типы** используются (`Part`, `PartType`)

Протобаф не привязан к языку — один `.proto` файл может сгенерировать код для Go, Python, Java и т.д.

### Шаг 2: Генерация Go-кода (task proto:gen)

Команда:
```bash
task proto:gen
```

Что происходит внутри:
1. Taskfile вызывает `buf generate` из директории `shared/proto/`
2. `buf` читает `buf.gen.yaml` — конфиг, где указано какие плагины использовать
3. Два плагина генерируют два файла в `shared/pkg/proto/inventory/v1/`:

| Плагин | Файл | Что генерирует |
|--------|------|----------------|
| `protoc-gen-go` | `inventory.pb.go` | Go-структуры для всех `message` и `enum` |
| `protoc-gen-go-grpc` | `inventory_grpc.pb.go` | Интерфейсы клиента и сервера |

### Что внутри сгенерированных файлов

**`inventory.pb.go`** — структуры данных:
```go
// Из message Part в .proto стала Go-структура:
type Part struct {
    Uuid          string
    Name          string
    Price         int64
    PartType      PartType
    StockQuantity int64
    CreatedAt     *timestamppb.Timestamp
    // ... (+ внутренние поля protobuf)
}

// Из enum PartType стали константы:
const (
    PartType_PART_TYPE_UNSPECIFIED PartType = 0
    PartType_PART_TYPE_HULL       PartType = 1
    PartType_PART_TYPE_ENGINE     PartType = 2
    PartType_PART_TYPE_SHIELD     PartType = 3
    PartType_PART_TYPE_WEAPON     PartType = 4
)
```

**`inventory_grpc.pb.go`** — интерфейсы:
```go
// Интерфейс СЕРВЕРА — это то, что ты реализуешь:
type InventoryServiceServer interface {
    GetPart(context.Context, *GetPartRequest) (*GetPartResponse, error)
    ListParts(context.Context, *ListPartsRequest) (*ListPartsResponse, error)
}

// Интерфейс КЛИЕНТА — это то, что Order будет использовать:
type InventoryServiceClient interface {
    GetPart(ctx context.Context, in *GetPartRequest, ...) (*GetPartResponse, error)
    ListParts(ctx context.Context, in *ListPartsRequest, ...) (*ListPartsResponse, error)
}

// Заглушка — встраивается в твой server, чтобы новые методы
// не ломали компиляцию (возвращают Unimplemented по умолчанию):
type UnimplementedInventoryServiceServer struct{}

// Функция регистрации сервера:
func RegisterInventoryServiceServer(s grpc.ServiceRegistrar, srv InventoryServiceServer)
```

**Эти файлы никогда не редактируются руками.** Если нужно изменить API — меняешь `.proto` и перегенерируешь.

### Шаг 3: Реализация сервера (service.go)

Файл: `inventory/pkg/service/service.go`

Здесь ты пишешь **бизнес-логику** — реализуешь интерфейс `InventoryServiceServer`.

#### Структура server

```go
type server struct {
    inventoryv1.UnimplementedInventoryServiceServer  // встраиваем заглушку
    parts map[uuid.UUID]Part                         // хранилище в памяти
}
```

- `UnimplementedInventoryServiceServer` — заглушка из сгенерированного кода. Без неё
  компилятор потребует реализовать ВСЕ методы интерфейса сразу. С ней — можно реализовывать
  по одному, остальные вернут `Unimplemented`.
- `parts` — map, где ключ = UUID, значение = деталь. На неделе 1 БД не используется.

#### Внутренний Part vs protobuf Part

В коде есть ДВА типа Part — это сделано намеренно:

```
service.Part (внутренний)          inventoryv1.Part (protobuf)
┌──────────────────────┐           ┌──────────────────────┐
│ UUID: string         │           │ Uuid: string         │
│ Name: string         │    ──►    │ Name: string         │
│ Price: int64         │  partTo   │ Price: int64         │
│ PartType: PartType   │  Proto()  │ PartType: PartType   │
│ StockQuantity: int64 │           │ StockQuantity: int64 │
│ CreatedAt: Timestamp │           │ CreatedAt: Timestamp │
└──────────────────────┘           └──────────────────────┘
   хранится в map                    отправляется по сети
```

Функция `partToProto()` конвертирует один в другой. Зачем разделять:
- Внутренний тип можно менять свободно (добавить поля, изменить логику)
- Protobuf тип — это контракт с другими сервисами, он зафиксирован

#### NewServer() — конструктор с seed-данными

```go
func NewServer() *server {
    return &server{
        parts: map[uuid.UUID]Part{
            uuid.MustParse("550e8400-...-0001"): {Name: "Алюминиевый корпус", ...},
            uuid.MustParse("550e8400-...-0002"): {Name: "Титановый корпус", ...},
            // ... всего 7 деталей
        },
    }
}
```

`uuid.MustParse` — как `uuid.Parse`, но паникует при ошибке (вместо возврата error).
Используется только для хардкода, где UUID заведомо валидный.

#### GetPart — получить деталь по UUID

```go
func (s *server) GetPart(ctx context.Context, req *inventoryv1.GetPartRequest) (*inventoryv1.GetPartResponse, error)
```

Путь выполнения:
```
Запрос: {uuid: "550e8400-...-0001"}
    │
    ├─ uuid пустой?          → return INVALID_ARGUMENT
    ├─ uuid невалидный?      → return INVALID_ARGUMENT
    ├─ нет в map?            → return NOT_FOUND
    └─ нашли                 → return GetPartResponse{Part: partToProto(part)}
```

- `req.GetUuid()` — безопасный getter (не паникует на nil)
- `uuid.Parse()` — проверяет формат UUID (например "abc" → ошибка)
- `s.parts[id]` — поиск в map, `ok` = true/false
- `status.Error(codes.X, "текст")` — gRPC-ошибка с кодом

#### ListParts — список деталей

Два режима работы:

```
Запрос: {uuids: ["0001", "0003"]}     Запрос: {part_type: PART_TYPE_ENGINE}
    │                                       │
    ├─ uuids не пустой                      ├─ uuids пустой
    │   │                                   │   │
    │   ├─ для каждого uuid:                │   ├─ для каждой детали в map:
    │   │   ├─ невалидный? → INVALID_ARG    │   │   └─ тип совпадает? → добавить
    │   │   ├─ не найден?  → NOT_FOUND      │   │     (UNSPECIFIED = все)
    │   │   └─ найден      → добавить       │   │
    │   │                                   │   ├─ сортировать по имени
    │   └─ вернуть (порядок = как в запросе) │   └─ вернуть
```

Ключевые отличия двух режимов:
- **По UUID**: порядок результата = порядок запроса, `part_type` игнорируется
- **По типу**: результат сортируется по имени (`sort.Slice`)

### Шаг 4: Запуск сервера (main.go)

Файл: `inventory/cmd/main.go`

```go
func main() {
    // 1. Создаём TCP-listener на порту :50051
    lis, _ := net.Listen("tcp", ":50051")

    // 2. Создаём gRPC-сервер
    grpcServer := grpc.NewServer()

    // 3. Регистрируем нашу реализацию
    inventoryv1.RegisterInventoryServiceServer(grpcServer, inventoryService.NewServer())

    // 4. Включаем reflection (чтобы grpcurl видел методы без .proto файла)
    reflection.Register(grpcServer)

    // 5. Запускаем — сервер начинает слушать входящие gRPC-вызовы
    grpcServer.Serve(lis)
}
```

Что происходит при `grpcServer.Serve(lis)`:
1. Сервер ждёт входящих TCP-соединений на порту 50051
2. Клиент (Order или grpcurl) подключается и отправляет бинарный gRPC-запрос
3. gRPC-фреймворк десериализует запрос в `*GetPartRequest` или `*ListPartsRequest`
4. Вызывает твой метод (`s.GetPart(...)` или `s.ListParts(...)`)
5. Ты возвращаешь Response — фреймворк сериализует его обратно в бинарный формат
6. Ответ уходит клиенту

---

## 3. Полный путь запроса (пример)

```
grpcurl -d '{"uuid": "...0001"}' localhost:50051 .../GetPart
    │
    │  1. grpcurl формирует бинарный gRPC-запрос (protobuf)
    │
    ▼
net.Listen("tcp", ":50051")
    │
    │  2. TCP-соединение установлено
    │
    ▼
grpc.Server
    │
    │  3. gRPC-фреймворк парсит запрос:
    │     - какой сервис? → InventoryService
    │     - какой метод?  → GetPart
    │     - тело запроса  → десериализует в *GetPartRequest{Uuid: "...0001"}
    │
    ▼
s.GetPart(ctx, &GetPartRequest{Uuid: "550e8400-...-0001"})
    │
    │  4. Твой код:
    │     - uuid.Parse("550e8400-...-0001") → ok
    │     - s.parts[id] → Part{Name: "Алюминиевый корпус", Price: 500000}
    │     - partToProto(part) → *inventoryv1.Part{...}
    │
    ▼
return &GetPartResponse{Part: &inventoryv1.Part{...}}, nil
    │
    │  5. gRPC-фреймворк сериализует ответ в protobuf (бинарный)
    │
    ▼
grpcurl получает и выводит как JSON:
{
  "part": {
    "uuid": "550e8400-...-0001",
    "name": "Алюминиевый корпус",
    "price": "500000",
    "partType": "PART_TYPE_HULL",
    "stockQuantity": "10"
  }
}
```

---

## 4. Структура файлов

```
inventory/
├── cmd/
│   └── main.go                        ← точка входа, запуск gRPC-сервера
├── pkg/
│   └── service/
│       └── service.go                 ← бизнес-логика (GetPart, ListParts)
├── go.mod                             ← зависимости модуля
└── go.sum                             ← контрольные суммы зависимостей

shared/
├── proto/
│   └── inventory/v1/
│       └── inventory.proto            ← КОНТРАКТ (ты пишешь)
│   buf.gen.yaml                       ← конфиг генерации (какие плагины)
│   buf.yaml                           ← конфиг линтинга proto
└── pkg/
    └── proto/
        └── inventory/v1/
            ├── inventory.pb.go        ← СГЕНЕРИРОВАНО (структуры)
            └── inventory_grpc.pb.go   ← СГЕНЕРИРОВАНО (интерфейсы)
```

---

## 5. Команды

| Команда | Что делает | Когда использовать |
|---------|------------|--------------------|
| `task proto:gen` | Генерирует Go-код из .proto | После изменения .proto файлов |
| `go run ./inventory/cmd` | Запускает сервис | Для ручного тестирования |
| `grpcurl -plaintext -d '{...}' localhost:50051 inventory.v1.InventoryService/GetPart` | Вызывает метод | Для проверки ответа |
| `task lint` | Проверяет код линтером | Перед коммитом |
| `task format` | Форматирует код | Перед коммитом |
| `task test:api` | Запускает тесты | Когда все 3 сервиса готовы |

---

## 6. Ключевые понятия

| Понятие | Что это | Где в коде |
|---------|---------|------------|
| **protobuf** | Бинарный формат сериализации от Google | `.proto` файлы |
| **gRPC** | Протокол вызова удалённых процедур поверх HTTP/2 | `grpc.NewServer()`, `grpc.Dial()` |
| **buf** | Инструмент для работы с protobuf (генерация, линтинг) | `bin/buf`, `buf.gen.yaml` |
| **reflection** | Сервис, позволяющий клиентам узнать API без .proto | `reflection.Register()` |
| **Unimplemented** | Заглушка — метод возвращает ошибку "не реализовано" | `UnimplementedInventoryServiceServer` |
| **status.Error** | Создание gRPC-ошибки с кодом (NotFound, InvalidArgument) | `status.Error(codes.NotFound, "...")` |
| **seed-данные** | Предзагруженные данные для тестирования | `NewServer()` |
