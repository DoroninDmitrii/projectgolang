# `ctx context.Context` — 7 кейсов «сломай и посмотри»

Про первый аргумент в [pkg/handler/handler.go:150](pkg/handler/handler.go#L150):

```go
func (h *handler) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (orderv1.CreateOrderRes, error) {
```

Про замок — соседний файл [MUTEX.md](MUTEX.md).

## Откуда ctx берётся (30 секунд чтения)

Ты его не создавал — тебе его дали:

```
net/http     создаёт r.Context() на каждый запрос, рвёт его при обрыве соединения
   ↓
ogen         oas_router_gen.go:72   → handleCreateOrderRequest(...)
             oas_handlers_gen.go:197 → ctx, span := s.cfg.Tracer.Start(r.Context(), ...)
             oas_handlers_gen.go:205 → ctx = contextWithLabeler(ctx, labeler)
             oas_handlers_gen.go:306 → s.h.CreateOrder(ctx, request)
   ↓
твой код     handler.go:163 → h.inventoryClient.ListParts(ctx, ...)
```

Проверить самому: `grep -n "r.Context()" ../shared/pkg/openapi/order/v1/oas_handlers_gen.go`

Контексты образуют **дерево**, и отмена идёт **только вниз**: оборвался HTTP-запрос —
отвалился и gRPC-вызов внутри него. Наоборот не работает.

## Как проверять

Эти кейсы про поведение клиента, поэтому нужны живые сервисы — три терминала:

```bash
cd project/inventory && go run ./cmd   # :50051
cd project/payment   && go run ./cmd   # :50052
cd project/order     && go run ./cmd   # :8080
```

Запрос:
```bash
curl -s -X POST localhost:8080/api/v1/orders -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440001","engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}'
```

Кейс 4 гоняется тестом, сервисы поднимать не надо — тест уже лежит в
[tests/demo_test.go](tests/demo_test.go) и собирается с тегом `demo`:

```bash
cd /Users/doronin/Personal/projectgolang/project/order
go test -tags demo ./tests/ -run TestDemoClientGoesAway -count=1 -v
```

> **`stat .../tests/tests: directory not found`** — ты внутри папки `tests/`, а `./tests/`
> считается от текущей директории. Либо `cd` в `project/order`, либо запускай с точкой:
> `go test -tags demo . -run TestDemoClientGoesAway -count=1 -v`.
> Забыл `-tags demo` → `[no tests to run]`.

Правки откатывай вручную — `git checkout` не используй, в дереве твои незакоммиченные изменения.

| Кейс | Что делаем | Что увидишь |
|------|-----------|-------------|
| 1 | залогировать ctx | свой ctx на каждый запрос, дедлайна нет |
| 2 | слушать `ctx.Done()` | `context canceled` при обрыве curl |
| 3 | `time.Sleep` вместо `select` | сервер не заметил ухода клиента |
| 4 | `context.Background()` вместо `ctx` | цепочка отмены порвана |
| 5 | `WithTimeout` вокруг gRPC | `DeadlineExceeded` → HTTP 500 |
| 6 | request-id через `ctx.Value` | сквозной id в логах |
| 7 | почему `GetOrder` игнорирует ctx | без правок, просто разбор |

---

## Кейс 1. Посмотреть, что внутри ctx

**Файл:** [handler.go:150](pkg/handler/handler.go#L150), первые строки `CreateOrder`

**СТАЛО:**
```go
func (h *handler) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (orderv1.CreateOrderRes, error) {
	deadline, hasDeadline := ctx.Deadline()
	slog.Info("старт CreateOrder",
		"адрес_ctx", fmt.Sprintf("%p", ctx),
		"ctx.Err()", ctx.Err(),
		"есть_дедлайн", hasDeadline,
		"дедлайн", deadline,
	)

	uuids := []string{req.HullUUID.String(), req.EngineUUID.String()}
```
Импорты: `"fmt"`, `"log/slog"`.

**ЗАПУСТИ:** два curl подряд.

**УВИДИШЬ** (настоящий вывод):
```
INFO старт ctx=0xb4c40140050 err=<nil>
INFO старт ctx=0xb4c401ec190 err=<nil>          ← адрес ДРУГОЙ
INFO дедлайн есть=false когда=0001-01-01T00:00:00.000Z
INFO дедлайн есть=false когда=0001-01-01T00:00:00.000Z
```

**ПОЧЕМУ:**
- **Адреса разные** — ctx рождается на каждый запрос заново. Это не глобальный объект,
  а «пульт» конкретного запроса. Отсюда правило: **ctx нельзя класть в поле структуры** —
  `handler` живёт всё время работы сервиса, а ctx умирает вместе с запросом.
- **`ctx.Err() == nil`** — контекст жив.
- **Дедлайна нет**, хотя в [cmd/main.go:36-40](cmd/main.go#L36-L40) стоят `ReadTimeout`,
  `WriteTimeout` и прочие. Это проверено отдельным тестом: таймауты `http.Server` работают
  на уровне TCP-соединения и в контекст не попадают. Нужен дедлайн в ctx — ставь сам (кейс 5).

---

## Кейс 2. Клиент ушёл — сервер узнал

**Файл:** [handler.go:163](pkg/handler/handler.go#L163), перед вызовом `ListParts`

**СТАЛО:**
```go
	slog.Info("делаю вид, что долго работаю")
	select {
	case <-time.After(10 * time.Second):
		slog.Info("доработал до конца")
	case <-ctx.Done():
		slog.Info("ОТМЕНА", "ctx.Err()", ctx.Err())
		return &orderv1.CreateOrderInternalServerError{
			Code: http.StatusInternalServerError, Message: "запрос отменён",
		}, nil
	}

	listResp, err := h.inventoryClient.ListParts(ctx, &inventoryv1.ListPartsRequest{
```

**ЗАПУСТИ:** `curl --max-time 2 ...` (или обычный curl и Ctrl+C через секунду).

**УВИДИШЬ** (настоящий вывод):
```
2026/08/12 17:49:22 INFO ОТМЕНА ctx.Err()="context canceled"
```

**ПОЧЕМУ:** обрыв TCP-соединения отменяет контекст запроса. `ctx.Done()` — это канал,
который при отмене **закрывается**; закрытый канал мгновенно отдаёт значение всем читателям
сразу, поэтому `select` тут же выбирает вторую ветку. Никаким другим способом сервер об уходе
клиента не узнает.

| Метод | пока всё хорошо | после отмены |
|-------|-----------------|--------------|
| `ctx.Err()` | `nil` | `context.Canceled` / `context.DeadlineExceeded` |
| `ctx.Done()` | канал открыт, чтение блокирует | канал закрыт, чтение мгновенно |

---

## Кейс 3. Отмена — это уведомление, а не выключатель

**СТАЛО** — тот же кейс 2, но `select` заменён на обычный сон:
```go
	slog.Info("делаю вид, что долго работаю")
	time.Sleep(10 * time.Second)
	slog.Info("доработал до конца")
```

**ЗАПУСТИ:** `curl --max-time 2 ...` — то же, что в кейсе 2.

**УВИДИШЬ:** через 10 секунд `доработал до конца`, заказ создан и записан в хранилище.
Клиента нет уже 8 секунд, а сервис всё ещё делает работу, результат которой никому не нужен.

**ПОЧЕМУ:** отмена контекста никого не «убивает» — она лишь закрывает канал. Если твой код
этот канал не читает, он спокойно доработает до конца. Именно поэтому ctx полезен только
там, где его кто-то слушает: `select`, gRPC-клиент, драйвер БД, `http.Client`.

---

## Кейс 4. `context.Background()` вместо `ctx` — рвём цепочку

Самый практичный кейс: именно так ctx ломают в реальном коде («компилятор ругался — я подставил
Background»).

**Файл:** [handler.go:163](pkg/handler/handler.go#L163)

**БЫЛО:**
```go
listResp, err := h.inventoryClient.ListParts(ctx, &inventoryv1.ListPartsRequest{
```

**СТАЛО:**
```go
listResp, err := h.inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
```

**Чтобы увидеть разницу**, во временно замедли Inventory —
[inventory/pkg/service/service.go](../inventory/pkg/service/service.go), начало `ListParts`:
```go
) (*inventoryv1.ListPartsResponse, error) {
	slog.Info("INVENTORY: начал работу, сплю 3 секунды")
	select {
	case <-time.After(3 * time.Second):
		slog.Info("INVENTORY: доспал до конца", "ctx.Err()", ctx.Err())
	case <-ctx.Done():
		slog.Info("INVENTORY: МЕНЯ ОТМЕНИЛИ", "ctx.Err()", ctx.Err())
		return nil, ctx.Err()
	}
```
Импорт: `"time"`.

**ЗАПУСТИ** — тест `TestDemoClientGoesAway` (клиент уходит через 500 мс) уже готов
в [tests/demo_test.go](tests/demo_test.go):
```bash
cd project/order
go test -tags demo ./tests/ -run TestDemoClientGoesAway -count=1 -v
```

**УВИДИШЬ** — два настоящих прогона рядом:

```
С ctx (как сейчас в коде):
  18:05:26 INFO INVENTORY: начал работу, сплю 3 секунды
  КЛИЕНТ ушёл через 500ms, err=... context deadline exceeded
  18:05:27 INFO INVENTORY: МЕНЯ ОТМЕНИЛИ ctx.Err()="context canceled"
                ↑ через секунду Inventory бросил работу

С context.Background():
  18:05:44 INFO INVENTORY: начал работу, сплю 3 секунды
  КЛИЕНТ ушёл через 500ms, err=... context deadline exceeded
  18:05:47 INFO INVENTORY: доспал до конца ctx.Err()=<nil>
                ↑ все 3 секунды молотил впустую, ctx даже не знает об отмене
```

**ПОЧЕМУ:** `context.Background()` — это **корень** дерева, у него нет родителя и он никогда
не отменяется. Подставив его, ты отцепил gRPC-вызов от запроса: клиента нет, а работа идёт.
Под нагрузкой такие «осиротевшие» вызовы копятся и укладывают соседний сервис.

Заодно видно, что дедлайн **уезжает по сети**: gRPC кладёт его в метаданные (`grpc-timeout`),
и Inventory получает уже свой ctx с тем же сроком — отмена работает через всю цепочку сервисов.

**ВЕРНИ:** `ctx` в вызов и убери задержку в Inventory.

---

## Кейс 5. Свой таймаут на вызов соседа

**Файл:** [handler.go:163](pkg/handler/handler.go#L163)

**СТАЛО:**
```go
	callCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	listResp, err := h.inventoryClient.ListParts(callCtx, &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	if err != nil {
		st, _ := status.FromError(err)
		slog.Info("gRPC вернул ошибку", "код", st.Code(), "текст", st.Message())
		switch st.Code() {
```
(задержка в Inventory из кейса 4 должна остаться — иначе вызов успеет уложиться)

**УВИДИШЬ:** `код=DeadlineExceeded`, клиенту уходит **HTTP 500**.

**ПОЧЕМУ:** `WithTimeout` создал **потомка** твоего ctx со своим сроком. Через миллисекунду
его `Done()` закрылся, gRPC оборвал вызов и вернул `codes.DeadlineExceeded`. Родительский `ctx`
жив — отмена идёт только вниз, поэтому ты спокойно формируешь ответ.

500 получается потому, что в твоём `switch` ([handler.go:168-175](pkg/handler/handler.go#L168-L175))
для этого кода ветки нет и он падает в `default`. В проде обычно различают — вот как это
выглядело бы (в код сейчас **не вносим**):

```go
case codes.DeadlineExceeded, codes.Canceled:
	// сосед не ответил вовремя — это 504, а не 500: проблема не в нас
	return &orderv1.CreateOrderInternalServerError{
		Code: http.StatusGatewayTimeout, Message: "InventoryService не ответил вовремя",
	}, nil
```

`defer cancel()` обязателен: без него контекст-потомок останется висеть в списке детей
родителя вместе с таймером — это утечка, `go vet` ругается на неё отдельно.

---

## Кейс 6. Сквозной request-id через `ctx.Value`

**Файл:** [cmd/main.go](cmd/main.go), перед созданием сервера

**СТАЛО:**
```go
type requestIDKey struct{}   // приватный тип-ключ, не строка

func requestIDMiddleware(req middleware.Request, next middleware.Next) (middleware.Response, error) {
	id := req.Raw.Header.Get("X-Request-Id")
	if id == "" {
		id = uuid.NewString()
	}
	req.SetContext(context.WithValue(req.Context, requestIDKey{}, id))
	return next(req)
}
```
Импорт: `"github.com/ogen-go/ogen/middleware"`.

`SetupServer` опций не принимает ([handler.go:93-95](pkg/handler/handler.go#L93-L95)),
поэтому для опыта собери сервер напрямую:
```go
orderServer, err := orderv1.NewServer(h, orderv1.WithMiddleware(requestIDMiddleware))
```

Достаём в хендлере (ключ должен быть виден обоим файлам — проще объявить его в пакете
`handler` как экспортируемый `RequestIDKey`):
```go
if id, ok := ctx.Value(handler.RequestIDKey{}).(string); ok {
	slog.Info("создаю заказ", "request_id", id)
}
```

**ЗАПУСТИ:** `curl -H 'X-Request-Id: my-test-123' -X POST localhost:8080/api/v1/orders ...`

**УВИДИШЬ:** `request_id=my-test-123` в логе. Без заголовка — сгенерированный UUID.

**ПОЧЕМУ ключ — `struct{}`, а не строка `"request_id"`:** `ctx.Value` ищет по значению любого
типа. Со строковым ключом любая библиотека, положившая значение по той же строке, молча затрёт
твоё. Приватный тип, объявленный в твоём пакете, снаружи недоступен — коллизия невозможна.
Бонус: `struct{}` занимает 0 байт.

**Ограничение:** `ctx.Value` — только для сквозных метаданных запроса (request-id, трассировка,
локаль, аутентификация). Бизнес-параметры через него не передают: это невидимые аргументы,
которых не видит ни компилятор, ни читатель кода.

---

## Кейс 7. Почему `GetOrder` игнорирует ctx (без правок)

**Файл:** [handler.go:99](pkg/handler/handler.go#L99)

```go
func (h *handler) GetOrder(_ context.Context, params orderv1.GetOrderParams) (...) {
//                         ^ подчёркивание = «параметр есть, но он мне не нужен»
	h.store.mu.RLock()
	order, ok := h.store.orders[params.OrderUUID]
	h.store.mu.RUnlock()
```

**ПОЧЕМУ:** `GetOrder` не делает ни одного долгого вызова — только чтение из map за наносекунды.
Отменять там нечего, проверять `ctx.Err()` бессмысленно. Из сигнатуры ctx убрать нельзя:
интерфейс `orderv1.Handler` сгенерирован ogen'ом и требует его во всех методах.

Правило: **ctx нужен там, где код может ждать** — сеть, диск, sleep, канал. В `CreateOrder`
и `PayOrder` есть gRPC-вызовы, поэтому там ctx с именем; в `GetOrder` — подчёркивание.

---

## Шпаргалка

| Нужно | Как |
|-------|-----|
| проверить, жив ли запрос | `if ctx.Err() != nil { ... }` |
| прервать долгое ожидание | `select { case <-ctx.Done(): return ctx.Err(); case ...: }` |
| ограничить вызов по времени | `ctx, cancel := context.WithTimeout(ctx, 3*time.Second)` + `defer cancel()` |
| отменить вручную | `ctx, cancel := context.WithCancel(ctx)`, затем `cancel()` |
| прокинуть request-id | `context.WithValue(ctx, приватныйКлюч{}, id)` |
| корень (в `main`, в тестах) | `context.Background()` |

- ctx — всегда первый аргумент, всегда с именем `ctx`.
- Не хранить ctx в полях долгоживущих структур (кейс 1).
- Передавать полученный ctx дальше во всё, что может ждать. `context.Background()` в середине
  цепочки — почти всегда баг (кейс 4).
- `cancel` вызывать всегда, обычно `defer` — иначе утечка (кейс 5).
- `nil` вместо контекста не передавать; нет контекста — `context.Background()`.
- `ctx.Value` — только метаданные запроса, не бизнес-параметры (кейс 6).
