# Неделя 2 — Архитектура и тесты. Разбор и пошаговый план

> Конспект-ментор по второй неделе курса «Микросервисы на Go 3.0».
> Источник задания — `plan/Неделя_2_Архитектура_и_тесты_｜Editors_tabs_#1.html`.
> Здесь: что меняется по сравнению с неделей 1, куда переезжает каждый кусок
> твоего кода, где задание расходится с реальностью проекта и в каком порядке
> всё делать. Идём строго по шагам — один сервис за раз, до зелёных тестов.
>
> Теория с разобранными примерами — в
> [`microservices-3-0-main/examples/week_2/HOW_IT_WORKS.md`](../microservices-3-0-main/examples/week_2/HOW_IT_WORKS.md).
> Здесь на неё ссылаемся, а не повторяем.

> Краткий разбор завершённого Payment со ссылками на код и схемами:
> [Payment: карта файлов и вызовов](payment/FLOW.md).

---

## 0. Что мы вообще делаем (картина целиком)

Функционально **ничего не меняется**: те же три сервиса, те же контракты, те же порты,
те же API-тесты. Меняется **устройство кода внутри** каждого сервиса и появляются
**unit-тесты**.

На неделе 1 вся логика сервиса жила в одном файле:

```
inventory/pkg/service/service.go     ← хранилище + бизнес-логика + gRPC-хендлер + конвертация
payment/pkg/service/service.go       ← валидация + генерация UUID + gRPC-хендлер
order/pkg/handler/handler.go         ← store + mutex + вызовы gRPC + расчёт цены + HTTP-ответы
```

Такой код нельзя протестировать по частям: чтобы проверить расчёт цены заказа, нужно
поднять HTTP-сервер и два gRPC-сервиса. На неделе 2 режем его на слои так, чтобы
каждый кусок можно было проверить изолированно, подменив соседей моками.

```
                 ┌──────────────────────────────────────────┐
  HTTP / gRPC ──►│ API          internal/api/…/v1/          │  вход: proto/DTO → model,
                 │              валидация формата, коды     │  ошибки → HTTP/gRPC-коды
                 └───────────────────┬──────────────────────┘
                                     │ интерфейс XxxService (api/…/v1/deps.go)
                                     ▼
                 ┌──────────────────────────────────────────┐
                 │ Service      internal/service/…/         │  бизнес-правила,
                 │              только model, без proto     │  оркестрация
                 └──────────┬─────────────────────┬─────────┘
       интерфейс Repository │                     │ интерфейс Client
       (service/…/deps.go)  ▼                     ▼ (service/…/deps.go)
                 ┌────────────────────┐  ┌───────────────────────────┐
                 │ Repository         │  │ Client (только в Order)   │
                 │ map + RWMutex      │  │ обёртка над gRPC-клиентом │
                 │ хранит record.*    │  │ proto ↔ model, коды→errs  │
                 └────────────────────┘  └───────────────────────────┘
```

### Поток данных при создании заказа (из ТЗ)

```
HTTP-клиент
  │ POST /api/v1/orders
  ▼
API (order/v1)            converter: DTO → model.CreateOrderRequest
  │ orderService.Create(ctx, req)
  ▼
Service (order)
  │ inventoryClient.ListParts(ctx, uuids)
  ▼
Inventory Client          converter: model → proto → gRPC ListParts → proto → []model.Part
  │ (обратно в Service)
  ▼
Service                   проверить остатки, посчитать total_price, собрать model.Order
  │ orderRepo.Create(ctx, order)
  ▼
Repository                converter: model.Order → record.Order, положить в map
  │ (обратно)
  ▼
API                       converter: model.Order → DTO, 201 Created
```

Каждая стрелка вниз — это вызов **через интерфейс**. Именно в этих местах в тестах
встанут моки.

---

## 1. Главные правила сдачи

1. **Один PR.** В нём и рефакторинг, и тесты, и покрытие ≥40%. «Сначала слои, потом
   тесты» двумя PR — не примут: рефакторинг без тестов не проверить, тесты без слоёв
   не написать.
2. **Весь пользовательский текст — на русском**: `errors.New("...")`, `slog.Info("...")`,
   комментарии, **имена тест-кейсов в `t.Run("...")`**, сообщения в `fmt.Errorf("...")`.
   На английском — только идентификаторы (функции, переменные, пакеты, JSON-теги, пути).
3. **Покрытие ≥40%** для пакетов `internal/api/` и `internal/service/` (всех трёх сервисов
   вместе). Проверяет `task test:coverage`, падает, если меньше.
4. **CI зелёный**: `task lint`, `task test:coverage`, `task test:api`.
5. **Покрытие показано бейджем в `README.md`.**

---

## 2. Чего НЕ делать на этой неделе

❌ **Не тестировать конвертеры** — там нет логики, только перекладывание полей.
❌ **Не тестировать репозиторий** — это `map` в памяти; позже его тесты станут
   интеграционными (с настоящей БД).
❌ **Не подключать БД** — остаётся `map` (БД — неделя 3).
❌ **Не писать интеграционные тесты** — только unit с моками. API-тесты уже есть в заготовке.
❌ **Не делать несколько реализаций одного интерфейса** (stub + grpc, как в `layers/`) —
   одна реализация на интерфейс.
❌ **Не добавлять middleware в service-слой, не делать фабрики.**
❌ **Метрики, трейсы, авторизация** — недели 6–7.
❌ **Не использовать `testify/suite`** — он хранит моки в полях структуры, это гонка
   при параллельных подтестах (см. разбор примера 7 в HOW_IT_WORKS.md, раздел 9).

---

## 3. Ключевые идеи недели (коротко)

Подробно каждая разобрана в `examples/week_2/HOW_IT_WORKS.md` — там живые примеры.

### 3.1. Зависимости направлены внутрь

Service **не знает** ни про proto, ни про `map`, ни про gRPC-клиенты. Он знает только
свои интерфейсы из `deps.go` и доменные модели из `internal/model`. Если в пакете
`service/...` появился импорт `shared/pkg/proto/...` или `repository/...` — это ошибка
архитектуры.

### 3.2. Интерфейс объявляет тот, кто его использует (DIP)

```
service/order/deps.go          объявляет  type OrderRepository interface { ... }
repository/order/repository.go реализует   (и ничего не знает про интерфейс)
```

Сервис не импортирует репозиторий — репозиторий подставляется снаружи через
`order.New(repo, invClient, payClient)` в `pkg/app`. Ровно эти интерфейсы потом
сгенерирует mockery.

### 3.3. Три модели данных, два типа конвертеров

| Модель | Пример | Где живёт | Особенности |
|--------|--------|-----------|-------------|
| **Proto / DTO** | `*inventoryv1.Part`, `*orderv1.CreateOrderRequest` | только API и Client | сгенерированы, трогать нельзя |
| **Model** | `model.Part`, `model.Order` | Service (и все границы) | enum-ы — типизированные строки: `OrderStatus`, `PartType` |
| **Record** | `record.Part`, `record.Order` | только Repository | примитивы: `Status string`, `PaymentMethod *string` |

Конвертеры на каждой границе:

| Пакет | Направление |
|-------|-------------|
| `internal/converter/` | Proto/DTO ↔ Model (для API) |
| `internal/repository/converter/` | Model ↔ Record (для Repository) |
| `internal/client/grpc/.../converter/` | Proto ↔ Model (для Client, только в Order) |

⚠️ **Типичная ошибка из ТЗ:** хранить `model.Order` прямо в `map` репозитория.
Репозиторий хранит `record.Order` и конвертирует на входе и выходе — это готовит
к PostgreSQL на неделе 3, где record станет строкой таблицы.

### 3.4. Валидация — в service-слое

Формат UUID, допустимость enum — бизнес-правило, место в service. Репозиторий
**доверяет** сервису и ничего не перепроверяет.

### 3.5. Ошибки: правила по слоям

| Слой | Что делает с ошибкой | Пример |
|------|---------------------|--------|
| **Repository** | Одна операция — возвращает как есть. Sentinel — напрямую | `return model.Order{}, errs.ErrOrderNotFound` |
| **Service** | Ошибку зависимости оборачивает: **глагол-инфинитив + существительное** | `fmt.Errorf("получить заказ: %w", err)` |
| **Service** | Своё бизнес-правило — sentinel без обёртки | `return errs.ErrOrderAlreadyPaid` |
| **Service** | Обёртка sentinel-а — только если добавляет реальный контекст | `fmt.Errorf("деталь %s: %w", part.Name, errs.ErrOutOfStock)` |
| **Client** | gRPC-код → sentinel; остальное — обёртка | `codes.NotFound` → `errs.ErrPartNotFound` |
| **API** | `errors.Is` → код протокола | `ErrOrderNotFound` → 404 / `codes.NotFound` |

Антипаттерны (ловят на ревью):

- ❌ `"не удалось получить заказ"` → ✅ `"получить заказ"` (что это ошибка — и так понятно)
- ❌ `"получение заказа из репозитория"` → ✅ `"получить заказ"`
- ❌ `fmt.Errorf("валидация uuid: %w", errs.ErrInvalidUUID)` — обёртка без нового смысла
- ❌ `if errors.Is(err, errs.ErrX) { return errs.ErrX }` — sentinel и так в цепочке
- ❌ `%v` вместо `%w` — `errors.Is` в API перестанет находить sentinel, всё уйдёт в 500

`errors.Is` в **service** нужен только когда от вида ошибки зависит **бизнес-логика**
(как graceful degradation в `layers/`). Просто пробросить — хватает `%w`.

### 3.6. Тесты: parallel table-driven, мок внутри подтеста

Обязательная форма (пример 7 из HOW_IT_WORKS.md):

1. `t.Parallel()` в начале `TestXxx` **и** в начале каждого `t.Run`;
2. кейсы — слайс структур, цикл `for _, tc := range tests` + `t.Run(tc.name, ...)`;
3. **моки создаются внутри подтеста** — `mocks.NewOrderRepository(t)`;
4. `require` — для «ворот» (после него идёт обращение к результату), `assert` — для проверок.

### 3.7. Зачем `pkg/app`

API-тесты Order (`order/tests/api_test.go`) поднимают Inventory и Payment у себя в
процессе через `bufconn`. Это **другой Go-модуль**, а `internal/` из чужого модуля
импортировать нельзя. Поэтому у каждого сервиса появляется тонкий публичный
`pkg/app/app.go`, который:

- собирает зависимости (repo → service → api) — это и есть DI;
- регистрирует сервис на **переданном** `*grpc.Server` (или возвращает HTTP-хендлер);
- **не создаёт** `grpc.Server` — keepalive, reflection, порты остаются в `main`.

Заготовка тестов вызывает ровно такие функции — сигнатуры фиксированы (см. раздел 5).

---

## 4. Где мы сейчас: куда переезжает код недели 1

### Payment — `payment/pkg/service/service.go`

| Что сейчас | Куда переезжает |
|-----------|-----------------|
| `server` + `PayOrder(ctx, *PayOrderRequest)` | `internal/api/payment/v1/{api,pay}.go` |
| проверки `order_uuid` пустой / не UUID, метод `UNSPECIFIED` | `internal/service/payment/pay.go` → доменные ошибки |
| `uuid.New()` для транзакции + `slog.Info` | `internal/service/payment/pay.go` |
| `status.Error(codes.InvalidArgument, …)` | `internal/api/payment/v1/pay.go` (маппинг `errs` → коды) |
| `paymentv1.PaymentMethod` в логике | `internal/model/payment.go` (`model.PaymentMethod`) + `internal/converter/payment.go` |
| `NewServer()` в `cmd/main.go` | `pkg/app/app.go` → `RegisterServices`, `Interceptors` |

### Inventory — `inventory/pkg/service/service.go`

| Что сейчас | Куда переезжает |
|-----------|-----------------|
| `type Part` (с proto-типами внутри!) | `internal/model/part.go` (домен) + `internal/repository/record/part.go` (хранение) |
| `parts map[uuid.UUID]Part` + seed в `NewServer()` | `internal/repository/part/repository.go` (+ `sync.RWMutex`) |
| `partToProto` | `internal/converter/part.go` |
| `uuid.Parse` → `InvalidArgument` | `internal/service/part/{get,list}.go` → `errs.ErrInvalidUUID` |
| поиск по UUID с сохранением порядка, фильтр по типу | `internal/repository/part/repository.go` |
| `sort.Slice` по имени | `internal/service/part/list.go` (бизнес-правило) |
| `GetPart` / `ListParts` хендлеры | `internal/api/inventory/v1/{get,list}.go` |

### Order — `order/pkg/handler/handler.go`

| Что сейчас | Куда переезжает |
|-----------|-----------------|
| `Order`, `OrderStatus`, `PaymentMethod` | `internal/model/order.go` |
| `orderStore` (map + mutex) | `internal/repository/order/repository.go` + `record/order.go` |
| `h.inventoryClient.ListParts(...)` + таймаут + разбор gRPC-кодов | `internal/client/grpc/inventory/v1/client.go` |
| `h.paymentClient.PayOrder(...)` + `paymentMethodToProto` + `uuid.Parse(transaction)` | `internal/client/grpc/payment/v1/client.go` + `converter/payment.go` |
| цикл по деталям: остаток, сумма цены | `internal/service/order/create.go` |
| проверки статуса в Pay / Cancel | `internal/service/order/{pay,cancel}.go` |
| сборка `OrderDto` из `Order` | `internal/converter/order.go` |
| `switch st.Code()` → `CreateOrderNotFound` и т.д. | `internal/api/order/v1/*.go` через `errors.Is(err, errs.…)` |
| `NewOrderStore`, `NewHandler`, `SetupServer` | `pkg/app/app.go` → `NewHTTPHandler` |

---

## 5. Где ТЗ расходится с твоим проектом — решаем заранее

Это самое важное в гайде: здесь то, на чём иначе застрянешь посреди работы.

### 5.1. Имя модуля в заготовке — `github.com/student`

Во всех файлах заготовки (`.mockery.yaml`, `order/tests/*.go`) импорты вида
`github.com/student/order/...`. У тебя модуль —
`github.com/DoroninDmitrii/projectgolang/project/...`. После копирования — замена
(команда в Этапе 0).

### 5.2. `ogen.ErrNotFound` из ТЗ у тебя не существует

Пример в ТЗ (`return nil, ogen.ErrNotFound // HTTP 404`) — условный. Твой ogen
сгенерировал **типизированные ответы на каждую операцию**:

```go
type CreateOrderNotFound Error    // func (*CreateOrderNotFound) createOrderRes() {}
type PayOrderConflict Error       // func (*PayOrderConflict) payOrderRes() {}
...
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
```

Значит, в Order API маппинг ошибок выглядит так же, как на неделе 1, только ветвление
идёт по доменным ошибкам, а не по gRPC-кодам:

```go
switch {
case errors.Is(err, errs.ErrOrderNotFound):
	return &orderv1.PayOrderNotFound{Code: http.StatusNotFound, Message: errs.ErrOrderNotFound.Error()}, nil
case errors.Is(err, errs.ErrOrderAlreadyPaid), errors.Is(err, errs.ErrOrderCancelled):
	return &orderv1.PayOrderConflict{Code: http.StatusConflict, Message: …}, nil
default:
	return nil, err // → ogen ErrorHandler → 500
}
```

`Message` лучше брать из sentinel-а, а не `err.Error()`: полная цепочка
(`"получить заказ: заказ не найден"`) — внутренняя кухня, клиенту она не нужна.

### 5.3. Что такое `error_handler.go` в Order API

Когда хендлер возвращает `nil, err`, ogen зовёт `ErrorHandler`. Дефолтный
(`ogenerrors.DefaultErrorHandler`) пишет в тело `{"error_message": err.Error()}` —
то есть **выдаёт клиенту текст внутренней ошибки** и не совпадает со схемой `Error`.

Свой `ErrorHandler` в `internal/api/order/v1/error_handler.go`:

- код берёт из `ogenerrors.ErrorCode(err)` — так ошибки декодирования (кривой JSON,
  невалидный UUID в path, неизвестный `payment_method`) **остаются 400**, API-тесты
  на это рассчитывают;
- для 500 — логирует настоящую ошибку через `slog.ErrorContext` и отдаёт клиенту
  нейтральное `"внутренняя ошибка"`;
- пишет JSON по схеме `{"code": …, "message": …}`.

Подключается в `pkg/app`: `orderv1.NewServer(api, orderv1.WithErrorHandler(v1.ErrorHandler))`.

### 5.4. Что такое `interceptor/error.go` в Inventory и Payment

API-тесты создают серверы так: `grpc.NewServer(inventoryApp.Interceptors()...)`.
Значит, `Interceptors()` возвращает `[]grpc.ServerOption`. Что в нём — решаешь сам;
разумный вариант — **страховочный unary-интерсептор**:

```go
// если хендлер вернул не gRPC-статус, а «голую» ошибку —
// логируем её и отдаём клиенту codes.Internal без подробностей
resp, err := handler(ctx, req)
if err == nil {
	return resp, nil
}
if _, ok := status.FromError(err); ok {
	return resp, err // хендлер уже выбрал код — не трогаем
}
slog.ErrorContext(ctx, "необработанная ошибка", "method", info.FullMethod, "error", err)
return nil, status.Error(codes.Internal, "внутренняя ошибка")
```

Основной маппинг `errs` → `codes` при этом остаётся в API-слое, как в ТЗ.

### 5.5. Репозиторий Inventory: `string` или `uuid.UUID`?

В ТЗ противоречие: раздел «Валидация» показывает `repo.Get(ctx, uuid.UUID)`, а
контракт `PartRepository` — `Get(ctx, uuid string)` и `PartFilter.UUIDs []string`.

**Решение: держимся контракта (`string`)**, а `map` в репозитории ключуем строкой —
`map[string]record.Part`. Тогда:

- сервис делает `uuid.Parse` один раз и возвращает `ErrInvalidUUID` при ошибке;
- репозиторий ничего не парсит (антипаттерн «повторная валидация» не возникает);
- `PartFilter` остаётся как в контракте.

### 5.6. `IsValid` из контракта Payment не скомпилируется

```go
func (m *PaymentMethod) IsValid() bool {
	switch m {                   // m — указатель, константы — значения
	case PaymentMethodCard, ...  // ошибка компиляции: mismatched types
```

Пиши с получателем-значением: `func (m PaymentMethod) IsValid() bool`.

### 5.7. Где лежат тест-файлы: `tests/` внутри пакета

Эталонная структура ТЗ кладёт тесты в подпапку:
`internal/service/order/tests/create_test.go`, `internal/api/order/v1/tests/get_test.go`.
Это **отдельный пакет** (`package tests`) — тесты «чёрного ящика»: видят только
экспортированное (`order.New`, методы), а не внутренние функции.

Следствия:

- конструкторы должны быть экспортированы: `New(...)`;
- покрытие считается правильно только благодаря `-coverpkg` в `task test:coverage` —
  без него пакет `service/order` показал бы 0%, потому что тесты лежат в другом пакете.

Пример в тексте ТЗ использует `package order` рядом с кодом — тоже рабочий вариант.
Выбери один и держись его во всех трёх сервисах. Рекомендую `tests/` — как в структуре.

### 5.8. `task test` в твоём Taskfile сейчас = API-тесты

Сейчас `test` — алиас на `test:api`. В заготовке `test` — это unit-тесты с `-race`
(без `order/tests`). Задачу **заменяешь**, не дублируешь. `test:api` и `test:race`
оставляешь как есть — CI (`test-api.yml`) зовёт `task test:race`.

### 5.9. CI у тебя работает из подпапки `project/`

Твои workflow-файлы используют `working-directory: project` и кэш `project/bin`.
`test-unit.yml` из заготовки написан для корня репозитория — при копировании
поправь `working-directory`, путь кэша и путь к `coverage/coverage.out`
(см. Этап 5).

### 5.10. Не потеряй `order/tests/demo_test.go`

ТЗ говорит «замени папку `order/tests` целиком». Но там лежит твой
`demo_test.go` (кейсы из MUTEX.md и CONTEXT.md, тег `demo`). Он использует только
HTTP-хелперы (`createOrder`, `payOrder`, …) — в новой `api_test.go` они с теми же
именами. Сохрани его перед заменой и верни обратно (команда в Этапе 0).

### 5.11. Гонка «проверил-потом-записал» в Pay вернётся — и это нормально

На неделе 1 `PayOrder` держал `Lock` на всё время операции. После разделения на слои
`service.Pay` делает `repo.Get` → проверка статуса → `paymentClient.PayOrder` →
`repo.Update` — **четыре отдельных вызова**, между ними блокировки нет. Два
параллельных запроса оплаты оба увидят `PENDING_PAYMENT` и оба заплатят
(MUTEX.md, кейс 7 — `TestDemoDoublePay` это покажет).

На этой неделе это **допустимо** — блокировки и транзакции разбираются на неделе 5
(«Kafka, блокировки и асинхронная коммуникация»). Просто знай, что это не баг
рефакторинга, а цена разделения на слои, которую потом закрывают на уровне БД.

---

## 6. Пошаговый план

Порядок в трекере ТЗ: Inventory → Payment → Order. **Идём иначе: Payment → Inventory →
Order** — от простого к сложному:

| Сервис | Слои | Зависимости сервиса | Чему учит |
|--------|------|---------------------|-----------|
| **Payment** | api + service | нет (stateless) | слои, converter, errs, маппинг в gRPC-коды, тест без моков |
| **Inventory** | api + service + repository | `PartRepository` | record/model, первый мок, `setupMock` |
| **Order** | api + service + repository + 2 client | `OrderRepository`, `InventoryClient`, `PaymentClient` | client-обёртки, три мока, `mock.MatchedBy`, ogen |

Каждый сервис проходит полный цикл: **слои → `pkg/app` + `main` → моки → тесты →
`go build` зелёный**. Только потом следующий.

> `task test:api` будет красным (не соберётся) от первого сервиса до конца Order:
> новые API-тесты требуют `pkg/app` у всех трёх. Это ожидаемо. Промежуточную проверку
> делаем через `go build` и unit-тесты сервиса.

---

### Этап 0. Подготовка (трекер: 1)

**0.1. Сохранить demo-тест:**
```bash
cd ~/Personal/projectgolang/project
cp order/tests/demo_test.go /tmp/demo_test.go.bak
```

**0.2. Скопировать заготовки:**
```bash
B=../microservices-3-0-main/homeworks/week_2/boilerplates

cp $B/.mockery.yaml $B/.mockery_boilerplate_text.txt .
cp $B/README.md ../README.md          # бейдж должен быть в корне репозитория — его показывает GitHub

rm -rf order/tests
cp -r $B/order/tests order/tests
cp /tmp/demo_test.go.bak order/tests/demo_test.go
```

**0.3. Заменить `github.com/student` на свой модуль:**
```bash
grep -rl 'github.com/student' .mockery.yaml order/tests \
  | xargs sed -i '' 's#github.com/student/#github.com/DoroninDmitrii/projectgolang/project/#g'
grep -rn 'github.com/student' .mockery.yaml order/tests    # должно быть пусто
```
В `.mockery_boilerplate_text.txt` поменяй имя на своё.

**0.4. `.mockery.yaml` — пока закомментируй пакеты.** mockery падает, если пакета
ещё нет. Раскомментируешь блоки по мере готовности сервисов.

**0.5. Taskfile — вручную, не копированием** (раздел 5.8). Из заготовки
`homeworks/week_2/boilerplates/Taskfile.yaml` переносим:

| Что | Куда |
|-----|------|
| vars `MOCKERY_VERSION`, `MOCKERY`, `COVERAGE_DIR`, `COVERAGE_FILE`, `COVERAGE_THRESHOLD` | в `vars:` |
| `mockery:install`, `mocks:gen` | новые задачи |
| `test` | **заменить** текущий алиас |
| `coverage:clean`, `test:coverage`, `coverage:threshold`, `coverage:html`, `coverage:check` | новые задачи |
| `mockery:install` | добавить в `deps` задачи `setup` |

**0.6. `.gitignore`** — добавь `coverage/` (отчёты не коммитим).

**0.7. Зависимости для тестов** — в `inventory` и `payment` их ещё нет:
```bash
for m in inventory payment order; do (cd $m && go get github.com/stretchr/testify@v1.11.1 github.com/brianvoe/gofakeit/v7); done
```
(или просто пиши импорты, а потом `task deps:update`).

**0.8. Поставить mockery:** `task mockery:install`.

✅ **Готово, когда:** `task --list` показывает `mocks:gen`, `test:coverage`, `coverage:html`;
в `order/tests/` лежат `api_test.go`, `architecture_test.go`, `demo_test.go`, `testutil/`;
`grep -r github.com/student` пусто.

---

### Этап 1. Payment (трекер: 3, частично 5–8)

Самый маленький сервис: нет хранилища, нет зависимостей у service. Задача — выстроить
слои на минимальном примере и научиться маппить ошибки.

**Целевая структура:**
```
payment/
├── cmd/main.go                          ← использовать app.Interceptors + app.RegisterServices
├── internal/
│   ├── api/payment/v1/
│   │   ├── api.go                       type api struct; func New(svc PaymentService) *api
│   │   ├── deps.go                      type PaymentService interface
│   │   ├── pay.go                       PayOrder: proto → model → service → коды
│   │   └── tests/pay_test.go
│   ├── converter/payment.go             proto → model.PayRequest
│   ├── errors/errors.go                 package errs
│   ├── interceptor/error.go             страховочный интерсептор (5.4)
│   ├── model/payment.go                 PaymentMethod, PayRequest
│   └── service/payment/
│       ├── service.go                   type service struct{}; func New() *service
│       ├── pay.go                       валидация + uuid + slog
│       └── tests/pay_test.go
└── pkg/app/app.go                       Interceptors(), RegisterServices()
```
`pkg/service/` — удаляем. `service/payment/deps.go` **не нужен** (у сервиса нет зависимостей).

**1.1. `internal/model/payment.go`** — `PaymentMethod string` с пятью константами
(`UNSPECIFIED`, `CARD`, `SBP`, `CREDIT_CARD`, `INVESTOR_MONEY`), метод `IsValid()` с
получателем-значением (5.6) и `PayRequest{OrderUUID string; PaymentMethod PaymentMethod}`.

**1.2. `internal/errors/errors.go`**, `package errs`:
```go
ErrInvalidOrderUUID     = errors.New("неверный формат UUID заказа")
ErrInvalidPaymentMethod = errors.New("неверный метод оплаты")
```

**1.3. `internal/converter/payment.go`** — `PayOrderRequestToModel(*paymentv1.PayOrderRequest) model.PayRequest`.
Enum маппится через `map[paymentv1.PaymentMethod]model.PaymentMethod`; всё, чего нет
в map, → `model.PaymentMethodUnspecified` (дальше сервис его отклонит).

**1.4. `internal/service/payment/pay.go`** — готовый пример есть в ТЗ (раздел
«Пример реализации»). Порядок: пустой / не-UUID `OrderUUID` → `ErrInvalidOrderUUID`;
`!IsValid()` → `ErrInvalidPaymentMethod`; `uuid.NewString()`; `slog.InfoContext(ctx,
"оплата выполнена", …)`. Ошибки — sentinel напрямую (это собственные бизнес-правила).

**1.5. `internal/api/payment/v1/`:**
- `deps.go`: `Pay(ctx, req model.PayRequest) (string, error)`;
- `api.go`: структура встраивает `paymentv1.UnimplementedPaymentServiceServer`;
- `pay.go`: `converter` → `a.paymentService.Pay` → маппинг:

| Ошибка | gRPC-код |
|--------|----------|
| `ErrInvalidOrderUUID` | `InvalidArgument` |
| `ErrInvalidPaymentMethod` | `InvalidArgument` |
| любая другая | `Internal`, `"внутренняя ошибка"` |

**1.6. `internal/interceptor/error.go` + `pkg/app/app.go`:**
```go
func Interceptors() []grpc.ServerOption      // grpc.ChainUnaryInterceptor(interceptor.…)
func RegisterServices(grpcServer *grpc.Server) // svc := payment.New(); api := v1.New(svc); paymentv1.Register…
```

**1.7. `cmd/main.go`** — к keepalive-опциям добавить `app.Interceptors()...`, вместо
`paymentv1.RegisterPaymentServiceServer(grpcServer, paymentService.NewServer())` —
`app.RegisterServices(grpcServer)`. Всё остальное (keepalive, reflection, graceful
shutdown) остаётся в `main`.

**1.8. Моки:** раскомментируй в `.mockery.yaml` блок
`…/payment/internal/api/payment/v1` (`.*Service`) → `task mocks:gen` → появится
`internal/api/payment/v1/mocks/mock_payment_service.go`.

**1.9. Тесты service** (`internal/service/payment/tests/pay_test.go`) — **без моков**,
зависимостей нет. Кейсы из контракта:

| `t.Run` | Вход | Ожидаем |
|---------|------|---------|
| `"оплата картой"` | валидный UUID, `CARD` | `NoError`, `uuid.Parse(tx)` без ошибки |
| `"оплата через СБП"` | валидный UUID, `SBP` | то же |
| `"невалидный order_uuid"` | `"not-a-uuid"` | `ErrorIs(ErrInvalidOrderUUID)` |
| `"пустой order_uuid"` | `""` | `ErrorIs(ErrInvalidOrderUUID)` |
| `"метод оплаты не указан"` | `UNSPECIFIED` | `ErrorIs(ErrInvalidPaymentMethod)` |

**1.10. Тесты API** (`internal/api/payment/v1/tests/pay_test.go`) — мок `PaymentService`.
Здесь проверяем **не бизнес-логику** (её мок подменяет), а три вещи API-слоя:
конвертацию (мок ожидает правильный `model.PayRequest`), маппинг ошибок в коды,
формирование ответа.

| `t.Run` | `setupMock` | Ожидаем |
|---------|-------------|---------|
| `"успешная оплата"` | `EXPECT().Pay(ctx, model.PayRequest{…, PaymentMethodCard}).Return(txUUID, nil)` | `res.GetTransactionUuid() == txUUID` |
| `"невалидный uuid заказа"` | `Return("", errs.ErrInvalidOrderUUID)` | `codes.InvalidArgument` |
| `"невалидный метод"` | `Return("", errs.ErrInvalidPaymentMethod)` | `codes.InvalidArgument` |
| `"внутренняя ошибка сервиса"` | `Return("", gofakeit.Error())` | `codes.Internal` |

✅ **Готово, когда:** `go build ./payment/...` и
`(cd payment && go test -race ./internal/...)` зелёные; в `payment/` нет импортов
`pkg/service`; в `internal/service/` нет импортов `shared/pkg/proto`.

---

### Этап 2. Inventory (трекер: 2, частично 5–7)

Появляется хранилище — а значит, record-модель, репозиторий и первый настоящий мок
в тестах service.

**Целевая структура:**
```
inventory/internal/
├── api/inventory/v1/     api.go, deps.go (PartService), get.go, list.go, tests/
├── converter/part.go     model ↔ proto, PartType ↔ proto enum, ListPartsRequest → PartFilter
├── errors/errors.go      ErrPartNotFound, ErrInvalidUUID
├── interceptor/error.go
├── model/part.go         Part, PartType, PartFilter
├── repository/
│   ├── converter/part.go record ↔ model
│   ├── record/part.go    PartType string, CreatedAt time.Time — без proto
│   └── part/repository.go map[string]record.Part + RWMutex + seed + Get + List
└── service/part/         deps.go (PartRepository), service.go, get.go, list.go, tests/
```
+ `pkg/app/app.go`, правка `cmd/main.go`, удалить `pkg/service/`.

**2.1. Модели.** `model.Part` — как в контракте (`CreatedAt time.Time`, **не**
`*timestamppb.Timestamp`). `PartType` — типизированная строка (`"HULL"`, `"ENGINE"` …,
`"UNSPECIFIED"`). `PartFilter{UUIDs []string; PartType PartType}`.
`record.Part` — те же поля, но `PartType string`.

**2.2. Репозиторий.** `New()` заполняет seed — **те же 7 деталей с теми же UUID,
ценами и остатками**, что сейчас в `NewServer()`: API-тесты сверяют их по константам
(`HullAluminumUUID`, `HullAluminumPrice`, `HullOutOfStockUUID` со stock=0 …).
Ключ map — строка (5.5).

- `Get(ctx, uuid string)` → нет в map → `errs.ErrPartNotFound` (sentinel как есть);
- `List(ctx, filter)`:
  - `len(filter.UUIDs) > 0` → детали **в порядке запрошенных UUID**, любой отсутствует →
    `ErrPartNotFound`, `PartType` игнорируется;
  - иначе → все детали, при `PartType != UNSPECIFIED` — только этого типа.
- Чтение — под `RLock`.

**2.3. Service.**
- `Get`: `uuid.Parse` → `ErrInvalidUUID`; `repo.Get` → `fmt.Errorf("получить деталь: %w", err)`.
- `List`: каждый UUID из фильтра — `uuid.Parse` → `ErrInvalidUUID`;
  `repo.List` → `fmt.Errorf("получить список деталей: %w", err)`;
  если фильтр **без** UUID — отсортировать по `Name` (`slices.SortFunc` + `strings.Compare`).
  Если UUID есть — **не сортировать** (порядок запроса — это контракт).

**2.4. API.** `GetPart` / `ListParts`: proto → model через converter → service →
маппинг → `converter.PartToProto`.

| Ошибка | gRPC-код |
|--------|----------|
| `ErrPartNotFound` | `NotFound` |
| `ErrInvalidUUID` | `InvalidArgument` |
| другая | `Internal` |

Пустой `uuid` в `GetPart` отдельно проверять не надо: `uuid.Parse("")` в сервисе даст
`ErrInvalidUUID` → `InvalidArgument`, API-тест `GetPart_EmptyUUID` это и ждёт.

**2.5. `pkg/app` + `cmd/main.go`** — как в Payment: `repo := part.New()` → `svc := …New(repo)`
→ `api := v1.New(svc)` → `inventoryv1.RegisterInventoryServiceServer`.

**2.6. Моки:** раскомментируй оба блока Inventory → `task mocks:gen` →
`mock_part_service.go` и `mock_part_repository.go`.

**2.7. Тесты service** (`service/part/tests/`) — мок `PartRepository`, поле
`setupMock func(repo *mocks.PartRepository)`:

| Метод | `t.Run` | Главное в кейсе |
|-------|---------|-----------------|
| Get | `"деталь найдена"` | `repo.EXPECT().Get(ctx, id).Return(part, nil)` |
| Get | `"деталь не найдена"` | репозиторий → `ErrPartNotFound`; ждём `ErrorIs` **сквозь обёртку** |
| Get | `"невалидный UUID"` | `setupMock` **пустой** — до репозитория дойти не должны; `ErrorIs(ErrInvalidUUID)` |
| Get | `"ошибка репозитория"` | `gofakeit.Error()` → `ErrorIs` этой же ошибки |
| List | `"без фильтра — все, по имени"` | репозиторий отдаёт в перемешанном порядке → результат отсортирован |
| List | `"фильтр по типу HULL"` | мок ожидает `PartFilter{PartType: HULL}` |
| List | `"по UUID — порядок сохранён"` | результат в порядке ответа репозитория, **не** отсортирован |
| List | `"один из UUID не найден"` | `ErrPartNotFound` |
| List | `"невалидный UUID в фильтре"` | репозиторий не вызван; `ErrInvalidUUID` |
| List | `"ошибка репозитория"` | ошибка пробрасывается |

Кейс «невалидный UUID» — самый ценный: он проверяет **правило слоёв** (валидация в
сервисе, репозиторий ей доверяет). Если по ошибке вызовешь репозиторий — мок упадёт
на неожиданном вызове.

**2.8. Тесты API** (`api/inventory/v1/tests/`) — мок `PartService`: успешный `GetPart`
(поля ответа совпадают), три ошибки → три кода; `ListParts` — мок ожидает правильный
`PartFilter` из proto-запроса (это тест конвертера «по пути», без отдельного теста конвертера).

✅ **Готово, когда:** `go build ./inventory/...`, `(cd inventory && go test -race ./internal/...)`
зелёные; в `internal/service` и `internal/model` нет импорта proto.

---

### Этап 3. Order (трекер: 4, частично 5–6, 9)

Самый насыщенный: две client-обёртки, три зависимости у сервиса, ogen в API.

**Целевая структура:**
```
order/internal/
├── api/order/v1/
│   ├── api.go, deps.go (OrderService)
│   ├── create.go, get.go, pay.go, cancel.go
│   ├── error_handler.go                        ErrorHandler для ogen (5.3)
│   └── tests/
├── client/grpc/
│   ├── inventory/v1/client.go + converter/inventory.go
│   └── payment/v1/client.go   + converter/payment.go
├── converter/order.go                          DTO ↔ model
├── errors/errors.go
├── model/order.go, model/part.go
├── repository/{converter,record,order}/
└── service/order/
    ├── deps.go (OrderRepository, InventoryClient, PaymentClient)
    ├── service.go, create.go, get.go, pay.go, cancel.go
    └── tests/
```
+ `pkg/app/app.go` (`NewHTTPHandler`), правка `cmd/main.go`, удалить `pkg/handler/`.

**3.1. Модели** — дословно из контракта ТЗ: `Order` (UUID-ы — `string`, опциональные —
`*string`), `OrderStatus`, `PaymentMethod`, `CreateOrderRequest` с методом `PartUUIDs()`,
`Part{UUID, Name, Price, StockQuantity}` — ровно то, что нужно заказу от Inventory.

**3.2. Ошибки:**
```go
ErrOrderNotFound    = errors.New("заказ не найден")
ErrOrderAlreadyPaid = errors.New("заказ уже оплачен")
ErrOrderCancelled   = errors.New("заказ отменён")
ErrPartNotFound     = errors.New("деталь не найдена")
ErrOutOfStock       = errors.New("деталь отсутствует на складе")
```

**3.3. Репозиторий** — `Create`, `Get`, `Update` над `map[string]record.Order` + `RWMutex`.
`Get`/`Update` несуществующего → `ErrOrderNotFound`. `record.Order.Status` — `string`,
`PaymentMethod` — `*string`.

**3.4. Inventory client** (`client/grpc/inventory/v1/client.go`):
```go
type client struct{ api inventoryv1.InventoryServiceClient }
func New(api inventoryv1.InventoryServiceClient) *client
func (c *client) ListParts(ctx context.Context, uuids []string) ([]model.Part, error)
```
Сюда переезжает `context.WithTimeout(ctx, inventoryCallTimeout)` из handler.go.
Ошибки: `codes.NotFound` → `errs.ErrPartNotFound`; остальное →
`fmt.Errorf("получить список деталей: %w", err)`.

**3.5. Payment client** (`client/grpc/payment/v1/client.go`):
`PayOrder(ctx, orderUUID string, method model.PaymentMethod) (string, error)` —
таймаут `paymentCallTimeout`, `model.PaymentMethod` → proto enum через converter,
проверка `uuid.Parse` ответа (как было в handler.go) →
`fmt.Errorf("разобрать transaction_uuid: %w", err)`; ошибку вызова оборачивает
`"оплатить заказ: %w"`.

**3.6. Service** — бизнес-правила из контракта:

| Метод | Правила | Ошибки |
|-------|---------|--------|
| `Create` | детали через `inventoryClient.ListParts(ctx, req.PartUUIDs())`; у каждой `StockQuantity > 0`; `TotalPrice` = сумма цен; `UUID = uuid.NewString()`; статус `PENDING_PAYMENT`; `CreatedAt = time.Now()`; `orderRepo.Create` | `"получить детали: %w"`; `fmt.Errorf("деталь %s: %w", part.Name, errs.ErrOutOfStock)`; `"сохранить заказ: %w"` |
| `Get` | `orderRepo.Get` | `"получить заказ: %w"` |
| `Pay` | `Get`; `PAID` → `ErrOrderAlreadyPaid`; `CANCELLED` → `ErrOrderCancelled`; `paymentClient.PayOrder`; статус `PAID`, сохранить `TransactionUUID` и `PaymentMethod`; `Update`; вернуть tx | обёртки: `"получить заказ"`, `"оплатить заказ"`, `"обновить заказ"` |
| `Cancel` | `Get`; `PAID` → `ErrOrderAlreadyPaid`; `CANCELLED` → `ErrOrderCancelled`; статус `CANCELLED`; `Update` | то же |

Полный пример `Create` и его теста — в ТЗ (раздел «Пример: Parallel Table-Driven Test»).

**3.7. API** — хендлеры ogen. Proto здесь нет, есть DTO из `shared/pkg/openapi/order/v1`:
`params.OrderUUID` — `uuid.UUID` → `.String()` для сервиса; обратно — `uuid.Parse`
в конвертере. Маппинг по разделу 5.2:

| Ошибка | HTTP | Тип ответа ogen |
|--------|------|-----------------|
| `ErrOrderNotFound` | 404 | `GetOrderNotFound`, `PayOrderNotFound`, `CancelOrderNotFound` |
| `ErrPartNotFound` | 404 | `CreateOrderNotFound` |
| `ErrOutOfStock` | 409 | `CreateOrderConflict` |
| `ErrOrderAlreadyPaid`, `ErrOrderCancelled` | 409 | `PayOrderConflict`, `CancelOrderConflict` |
| другая | 500 | `return nil, err` → `ErrorHandler` |

Ошибки формата (кривой JSON, невалидный UUID, неизвестный `payment_method`) до
хендлера не доходят — их отсекает ogen, и они должны остаться 400 (5.3).

**3.8. `pkg/app/app.go`** — сигнатура задана API-тестом:
```go
func NewHTTPHandler(
	inventoryClient inventoryv1.InventoryServiceClient,
	paymentClient paymentv1.PaymentServiceClient,
) (http.Handler, error)
```
Внутри: `repo := order.New()` → client-обёртки → `svc := …New(repo, inv, pay)` →
`api := v1.New(svc)` → `orderv1.NewServer(api, orderv1.WithErrorHandler(v1.ErrorHandler))`.
`cmd/main.go` создаёт gRPC-соединения (они — инфраструктура) и передаёт сгенерированные
proto-клиенты в `app.NewHTTPHandler`.

**3.9. Моки:** раскомментируй оба блока Order → `task mocks:gen` → 4 файла:
`mock_order_service.go`, `mock_order_repository.go`, `mock_inventory_client.go`,
`mock_payment_client.go`.

**3.10. Тесты service** (`service/order/tests/`) — все кейсы из контракта ТЗ:

| Метод | Кейсы (`t.Run`, по-русски) |
|-------|---------------------------|
| **Create** | успех с hull + engine · успех с shield и weapon · деталь не найдена (`ErrPartNotFound`) · деталь закончилась (`ErrOutOfStock`) · ошибка сохранения |
| **Get** | заказ найден · не найден (`ErrOrderNotFound`) · ошибка репозитория |
| **Pay** | успешная оплата · не найден · уже оплачен (`ErrOrderAlreadyPaid`) · отменён (`ErrOrderCancelled`) · ошибка PaymentService |
| **Cancel** | успешная отмена · не найден · уже оплачен · уже отменён |

Приёмы, которые здесь понадобятся:

- **`setupMock` с тремя моками**: `func(repo *mocks.OrderRepository, inv *mocks.InventoryClient, pay *mocks.PaymentClient)`;
- **`mock.MatchedBy`** для `repo.Create` / `repo.Update`: `UUID` и `CreatedAt` генерируются
  внутри сервиса — проверяем свойства (`o.UUID != ""`, `o.TotalPrice == 800000`,
  `o.Status == model.OrderStatusPendingPayment`), а не равенство;
- **в кейсах-ошибках не настраиваем «лишние» моки**: в «деталь закончилась» нет
  `repo.EXPECT().Create` — если сервис всё-таки сохранит заказ, тест упадёт.
  Это проверка того, что заказ **не создаётся** при нехватке детали;
- в «уже оплачен» у `pay` нет `EXPECT` — **деньги не списываются второй раз**.
  Это главный кейс всего сервиса.

**3.11. Тесты API** (`api/order/v1/tests/`) — мок `OrderService`. На каждый метод:
успех + каждая доменная ошибка → свой тип ответа ogen:
```go
res, err := a.PayOrder(ctx, req, params)
require.NoError(t, err)
assert.IsType(t, &orderv1.PayOrderConflict{}, res)
```
Плюс кейс «неизвестная ошибка» → `err != nil` (дальше её превратит в 500 `ErrorHandler`).

✅ **Готово, когда:** `task build`, `task test`, **`task test:api`** — всё зелёное.
`test:api` — первый момент, когда все три `pkg/app` существуют и API-тесты вообще
собираются. Если что-то красное — это расхождение поведения с неделей 1:
смотри, какой HTTP-код/gRPC-код ожидает тест, и ищи, в каком слое потерялся sentinel.

---

### Этап 4. Проверки качества (трекер: 10–12)

```bash
task format          # gofumpt + gci (моки и сгенерированный код не трогает)
task lint            # 0 замечаний
task mocks:gen       # моки актуальны (после любой правки интерфейса!)
task test            # unit-тесты с -race
task test:coverage   # покрытие ≥40%, иначе задача упадёт
task coverage:html   # открыть отчёт и посмотреть, что НЕ покрыто
task test:api        # API-тесты
```

Что обычно всплывает в линтере на этой неделе:

| Линтер | Что ловит | Что делать |
|--------|-----------|------------|
| `errorlint` | `err == errs.ErrX` | `errors.Is(err, errs.ErrX)` |
| `containedctx` | `ctx` в поле структуры | в тестах отключён; в коде — не храни ctx в структурах |
| `perfsprint` | `fmt.Errorf` без `%w`/аргументов | `errors.New` |
| `forbidigo` | `fmt.Print*` | `slog` |

Если покрытие не дотягивает до 40% — сначала добей **API-тесты на маппинг ошибок**:
они короткие и покрывают много веток `switch`.

---

### Этап 5. Покрытие в README и CI (трекер: 13, 15)

**5.1. CI: добавить unit-тесты.** Скопируй
`homeworks/week_2/boilerplates/.github/workflows/test-unit.yml` в
`../.github/workflows/test-unit.yml` и поправь под свою раскладку (5.9):

```yaml
jobs:
  test-unit:
    runs-on: ubuntu-24.04
    defaults:
      run:
        working-directory: project        # ← добавить
    steps:
      ...
      - uses: actions/cache@v5.0.5
        with:
          path: project/bin               # ← было bin
          key: ${{ runner.os }}-task-bin-${{ hashFiles('project/Taskfile.yaml') }}
```
В `ci.yml` добавь job `test-unit` (`needs: build`, `uses: ./.github/workflows/test-unit.yml`).

**5.2. Бейдж покрытия.** Шаблон `README.md` ссылается на gist:
`https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/<USER>/<GIST_ID>/raw/coverage.json`.
Нужно, чтобы CI при пуше в `main` записывал в этот gist JSON с процентом.

1. Создай **публичный** gist с любым файлом `coverage.json` — запомни его ID.
2. Создай Personal Access Token со scope **`gist`** и положи в секреты репозитория
   как `GIST_SECRET`.
3. В `test-unit.yml` после тестов:
   ```yaml
   - name: Посчитать процент покрытия
     id: cov
     run: echo "percent=$(go tool cover -func=coverage/coverage.out | tail -1 | awk '{print $NF}' | tr -d '%')" >> "$GITHUB_OUTPUT"

   - name: Обновить бейдж покрытия
     if: github.ref == 'refs/heads/main'
     uses: schneegans/dynamic-badges-action@v1.7.0
     with:
       auth: ${{ secrets.GIST_SECRET }}
       gistID: <GIST_ID>
       filename: coverage.json
       label: coverage
       message: ${{ steps.cov.outputs.percent }}%
       valColorRange: ${{ steps.cov.outputs.percent }}
       minColorRange: 40
       maxColorRange: 90
   ```
4. В корневом `README.md` подставь `<USER>` и `<GIST_ID>`.

Бейдж обновится после первого мержа в `main` — в PR он может показывать старое
значение, это нормально.

---

### Этап 6. PR (трекер: 14)

```bash
git checkout -b week-2
git add -A
git commit -m "неделя 2: слои Clean Architecture и unit-тесты"
git push -u origin week-2
gh pr create --title "Неделя 2 — Архитектура и тесты" --body "..."
```

Чек-лист перед созданием PR:

- [ ] в коде нет `pkg/service/` и `pkg/handler/` (старые пакеты удалены)
- [ ] в `internal/service/**` нет импортов `shared/pkg/proto` и `shared/pkg/openapi`
- [ ] в `internal/repository/**` в map лежат `record.*`, не `model.*`
- [ ] все `t.Run(...)` и сообщения ошибок — по-русски
- [ ] моки закоммичены (`task mocks:gen` → `git status` чистый)
- [ ] `task lint`, `task test:coverage`, `task test:api` — зелёные локально

---

## 7. Трекер (15 пунктов из ТЗ)

- [x] **1. Скопировать boilerplate в проект** — Этап 0.
  - Ветка `week-2`. `.mockery.yaml` (пакеты закомментированы до готовности сервисов), `.mockery_boilerplate_text.txt`, `README.md` в корне репозитория, новые `order/tests` (`demo_test.go` сохранён), префикс модуля заменён.
  - Taskfile: `mockery:install`, `mocks:gen`, `test` (теперь unit-тесты), `test:coverage`, `coverage:*`; `coverage/` в `.gitignore`; mockery v3.7.0 в `bin/`.
- [ ] **2. Выделить слои в InventoryService** — Этап 2.
- [x] **3. Выделить слои в PaymentService** — Этап 1, шаги 1.1–1.7.
  - Созданы model, errors, converter, service, API, interceptor и pkg/app; main использует новые слои, старый pkg/service удалён.
  - Проверено 7 октября 2026: `go build ./payment/...` и `go vet ./payment/...` проходят из каталога `project`.
- [ ] **4. Выделить слои в OrderService** — Этап 3.
- [ ] **5. Определить интерфейсы зависимостей** — `deps.go` в каждом `api/…/v1` и `service/…` (кроме Payment service).
  - Payment: интерфейс `PaymentService` готов. Inventory и Order — впереди.
- [ ] **6. Сгенерировать моки `task mocks:gen`** — после каждого сервиса.
  - Payment: блок в `.mockery.yaml` открыт, `mock_payment_service.go` создан, testify v1.11.1 добавлен в модуль. Inventory и Order — впереди.
- [ ] **7. Unit-тесты InventoryService** — Этап 2.7–2.8.
- [x] **8. Unit-тесты PaymentService** — Этап 1.9–1.10.
  - Восемь сценариев `service.Pay` и четыре сценария `api.PayOrder` с моком проходят с `-race` по результатам запуска пользователя 9 октября 2026. Сборка Payment также проходит.
- [ ] **9. Unit-тесты OrderService** — Этап 3.10–3.11.
- [ ] **10. `task lint` без ошибок** — Этап 4.
- [ ] **11. `task test:coverage` без ошибок** — Этап 4.
- [ ] **12. `task test:api` проходит** — конец Этапа 3.
- [ ] **13. Покрытие в README.md** — Этап 5.2.
- [ ] **14. PR с рефакторингом и тестами** — Этап 6.
- [ ] **15. CI зелёный** — Этап 5.1 + 6.

---

## 8. Шпаргалка

### Скелет теста (под нашу форму)

```go
func TestPay(t *testing.T) {
	t.Parallel()

	var (
		ctx       = context.Background()
		orderUUID = gofakeit.UUID()
		txUUID    = gofakeit.UUID()
	)

	tests := []struct {
		name      string
		setupMock func(repo *mocks.OrderRepository, pay *mocks.PaymentClient)
		wantErr   error
	}{
		{
			name: "заказ уже оплачен",
			setupMock: func(repo *mocks.OrderRepository, _ *mocks.PaymentClient) {
				repo.EXPECT().Get(ctx, orderUUID).
					Return(model.Order{UUID: orderUUID, Status: model.OrderStatusPaid}, nil)
				// pay.EXPECT() нет — второй раз списывать нельзя
			},
			wantErr: errs.ErrOrderAlreadyPaid,
		},
		// ...
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := mocks.NewOrderRepository(t)
			inv := mocks.NewInventoryClient(t)
			pay := mocks.NewPaymentClient(t)
			tc.setupMock(repo, pay)

			svc := order.New(repo, inv, pay)
			tx, err := svc.Pay(ctx, orderUUID, model.PaymentMethodCard)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, tx)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, txUUID, tx)
		})
	}
}
```

### Полезные приёмы mockery

```go
m.EXPECT().Get(ctx, id).Return(x, nil)                    // точные аргументы
m.EXPECT().Create(ctx, mock.MatchedBy(func(o model.Order) bool { … })).Return(nil)
m.EXPECT().Get(mock.Anything, id).Return(x, nil)           // ctx не важен (код внутри подменяет его, например WithTimeout)
m.AssertNotCalled(t, "Create")                             // явная проверка «не вызывали»
```

`mocks.NewXxx(t)` сам проверит в конце подтеста, что все `EXPECT` сработали, и уронит
тест на любом **неожиданном** вызове — отдельный `AssertExpectations` не нужен.

### Команды

| Команда | Когда |
|---------|-------|
| `task mocks:gen` | после любого изменения интерфейса в `deps.go` |
| `(cd <svc> && go test -race ./internal/...)` | быстрая проверка одного сервиса по ходу работы |
| `task test` | все unit-тесты |
| `task test:coverage` / `task coverage:html` | покрытие / что не покрыто |
| `task test:api` | интеграция всех трёх сервисов через bufconn + httptest |
| `task lint` / `task format` | перед коммитом |

---

## 9. Текущий прогресс и следующий шаг

Актуально на 9 октября 2026. Подготовка из Этапа 0 отмечена в общем трекере.

**Этап 1 — Payment:**

- [x] 1.1. Доменная модель: PaymentMethod, IsValid, PayRequest.
- [x] 1.2. Доменные ошибки.
- [x] 1.3. Конвертер protobuf → model.
- [x] 1.4. Бизнес-сервис Pay.
- [x] 1.5. API: интерфейс зависимости, конструктор, PayOrder.
- [x] 1.6. Интерсептор ошибок и сборка в pkg/app.
- [x] 1.7. Подключение в main и удаление старого pkg/service.
- [x] 1.8. Генерация мока PaymentService и подключение testify/mock.
- [x] 1.9. Unit-тесты бизнес-сервиса Pay.
- [x] 1.10. Unit-тесты API PayOrder.

Проверено 7 октября: `go vet ./payment/...` проходит из каталога `project`.
По результатам запуска пользователя 9 октября успешно выполнены
`go build ./payment/...` и `go test -race -count=1 ./payment/internal/...`:
восемь сценариев сервиса и четыре сценария API прошли, гонок в этом запуске
не обнаружено. **Этап 1 (Payment) завершён.**
Полные проверки недели (lint, покрытие, API-тесты) ещё не завершены.

**Следующий шаг — 2.1 (Inventory):** выделить доменные модели `Part`, `PartType`,
`PartFilter` в `inventory/internal/model/part.go` и модель хранения
`record.Part` в `inventory/internal/repository/record/part.go`.
Здесь появляется новый слой — репозиторий с хранением деталей в памяти.

Краткая карта завершённого Payment со схемами и ссылками: [payment/FLOW.md](payment/FLOW.md).
