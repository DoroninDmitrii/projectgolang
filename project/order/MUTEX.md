# Замок (`mu.Lock`) — 7 кейсов «сломай и посмотри»

Про эти три строки в [pkg/handler/handler.go:203-205](pkg/handler/handler.go#L203-L205):

```go
h.store.mu.Lock()
h.store.orders[order.OrderUUID] = order
h.store.mu.Unlock()
```

Про `ctx` — соседний файл [CONTEXT.md](CONTEXT.md).

## Как проверять

Тесты для всех кейсов уже лежат в [tests/demo_test.go](tests/demo_test.go) — писать ничего
не надо. Стенд поднимает `TestMain` из [tests/api_test.go](tests/api_test.go): Inventory
и Payment через bufconn, Order через httptest. Ни Docker, ни запущенные сервисы не нужны.

Файл собирается только с тегом `demo`, чтобы не мешать обычному `go test ./...`:

```bash
cd /Users/doronin/Personal/projectgolang/project/order
go test -tags demo ./tests/ -race -run TestDemoParallelCreate -count=1 -v
```

> **Если видишь `stat .../tests/tests: directory not found`** — ты находишься внутри папки
> `tests/`, а путь `./tests/` считается от текущей директории. Либо поднимись в
> `project/order` (команда выше), либо из `tests/` запускай с точкой:
> ```bash
> go test -tags demo . -race -run TestDemoParallelCreate -count=1 -v
> ```

Забыл `-tags demo` → увидишь `[no tests to run]`.

**Правки откатывай вручную** (Ctrl+Z или по диффу `git diff project/order/pkg/handler/handler.go`).
`git checkout` не используй — в рабочем дереве лежат твои незакоммиченные правки.

| Кейс | Что ломаем | Что увидишь |
|------|-----------|-------------|
| 1 | убрать `Lock`/`Unlock` | `WARNING: DATA RACE` |
| 2 | то же, без `-race` | `fatal error: concurrent map writes`, процесс убит |
| 3 | `RLock` вместо `Lock` | та же гонка — `RWMutex` не спасает |
| 4 | `sleep` внутри секции | второй запрос стоит в очереди |
| 5 | убрать запись обратно в `PayOrder` | оплата «прошла», статус остался `PENDING_PAYMENT` |
| 6 | `Lock()` дважды | сервис виснет навсегда |
| 7 | разбить `Lock` в `PayOrder` | заказ оплачен дважды |

---

## Кейс 1. Без замка — гонка

**Файл:** [handler.go:203-205](pkg/handler/handler.go#L203-L205)

**БЫЛО:**
```go
h.store.mu.Lock()
h.store.orders[order.OrderUUID] = order
h.store.mu.Unlock()
```

**СТАЛО:**
```go
h.store.orders[order.OrderUUID] = order
```

**ЗАПУСТИ** — тест `TestDemoParallelCreate` (30 параллельных POST) уже готов
в [tests/demo_test.go](tests/demo_test.go):
```bash
cd project/order
go test -tags demo ./tests/ -race -run TestDemoParallelCreate -count=1
```

**УВИДИШЬ** (настоящий вывод на этом проекте):
```
WARNING: DATA RACE
Write at 0x00c00027fbf0 by goroutine 137:
  runtime.mapaccess2()
  .../pkg/handler.(*handler).CreateOrder()
      .../pkg/handler/handler.go:203
  .../openapi/order/v1.(*Server).handleCreateOrderRequest()
  .../openapi/order/v1.(*Server).ServeHTTP()
  net/http.serverHandler.ServeHTTP()
  net/http.(*conn).serve()
  net/http.(*Server).Serve.gowrap3()          ← вот она, горутина на запрос

Previous write at 0x00c00027fbf0 by goroutine 138:
  ... тот же handler.go:203, другая горутина ...
--- FAIL: TestDemoParallelCreate
    race detected during execution of test
```

**ПОЧЕМУ:** `net/http` запускает **отдельную горутину на каждый запрос** (`gowrap3` в стеке).
`handler` и его `store` — одни на весь процесс ([cmd/main.go:74-79](cmd/main.go#L74-L79)),
значит 30 горутин пишут в одну map. Стек читается сверху вниз: сеть → ogen → твой код.

**ВЕРНИ:** `Lock`/`Unlock` на место.

---

## Кейс 2. То же самое, но без `-race` — процесс умирает

**СТАЛО:** то же, что в кейсе 1 (замок убран).

**ЗАПУСТИ** несколько раз подряд — тот же тест, но **без** `-race`:
```bash
cd project/order
go test -tags demo ./tests/ -run TestDemoParallelCreate -count=1
```

**УВИДИШЬ** (воспроизвелось 3 раза из 3):
```
fatal error: concurrent map writes

goroutine 217 [running]:
internal/runtime/maps.fatal(...)
	.../src/runtime/panic.go:1181
.../pkg/handler.(*handler).CreateOrder(...)
	.../pkg/handler/handler.go:203
```

**ПОЧЕМУ:** map в Go — хеш-таблица, которая при росте перекладывает элементы в новый массив.
Пока перекладывание идёт, структура несогласованна; вторая горутина в этот момент её ломает.
Рантайм ловит это и **убивает процесс целиком**. Ключевое: это `fatal error`, а **не** `panic` —
`recover()` не поможет, упадут заодно все остальные запросы.

Опасность в том, что детект вероятностный: на 30 горутинах вылезает, на 3 может не вылезти
ни разу за месяц — и доедет до прода. Поэтому тесты гоняй под `-race`.

---

## Кейс 3. `RLock` вместо `Lock` — не спасает

**Файл:** [handler.go:203](pkg/handler/handler.go#L203)

**БЫЛО:**
```go
h.store.mu.Lock()
h.store.orders[order.OrderUUID] = order
h.store.mu.Unlock()
```

**СТАЛО:**
```go
h.store.mu.RLock()
h.store.orders[order.OrderUUID] = order
h.store.mu.RUnlock()
```

**ЗАПУСТИ:** тот же тест с `-race`:
```bash
cd project/order
go test -tags demo ./tests/ -race -run TestDemoParallelCreate -count=1
```

**УВИДИШЬ** (проверено — результат тот же, что без замка вообще):
```
WARNING: DATA RACE
Write at 0x00c00027fbf0 by goroutine 163:
  .../pkg/handler/handler.go:204
...
fatal error: concurrent map writes    ← и без -race тоже падает
```

**ПОЧЕМУ:** `RLock` — это «читательский» замок: он пускает **много горутин одновременно**,
потому что параллельное чтение безопасно. Компилятор не проверяет, что ты под ним делаешь,
поэтому 30 «читателей» спокойно пишут в map хором. `RLock` годится только там, где ты
действительно ничего не меняешь — как в [GetOrder:101](pkg/handler/handler.go#L101).

| Что делаю | Что брать | Сколько горутин одновременно |
|-----------|-----------|------------------------------|
| читаю | `RLock` / `RUnlock` | много |
| пишу (в т.ч. в map) | `Lock` / `Unlock` | ровно одна |

---

## Кейс 4. Замок — это очередь

**Файл:** [handler.go:203-205](pkg/handler/handler.go#L203-L205)

**СТАЛО:**
```go
slog.Info("хочу взять замок", "заказ", order.OrderUUID)
h.store.mu.Lock()
slog.Info("ВЗЯЛ замок", "заказ", order.OrderUUID)
h.store.orders[order.OrderUUID] = order
time.Sleep(2 * time.Second)                       // держим замок подольше
h.store.mu.Unlock()
slog.Info("отпустил замок", "заказ", order.OrderUUID)
```
Добавь `"log/slog"` в импорты.

**ЗАПУСТИ** сервисы (`go run ./cmd` в inventory, payment, order) и два curl параллельно:
```bash
curl -s -X POST localhost:8080/api/v1/orders -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440001","engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}' &
curl -s -X POST localhost:8080/api/v1/orders -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440001","engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}' &
```

**УВИДИШЬ** (настоящий лог, A и B — два запроса):
```
17:48:37 INFO хочу взять замок q=A
17:48:37 INFO ВЗЯЛ замок q=A
17:48:37 INFO хочу взять замок q=B     ← B пришёл сразу за A
17:48:39 INFO отпустил замок q=A       ← через 2 секунды
17:48:39 INFO ВЗЯЛ замок q=B           ← B получил замок в ту же секунду
17:48:41 INFO отпустил замок q=B
```

**ПОЧЕМУ:** между «хочу» и «взял» у B горутина **припаркована** рантаймом: она не крутится
в цикле и не жжёт процессор, а поток ОС в это время выполняет другие горутины. Отсюда правило:
**критическая секция должна быть максимально короткой** — на эти 2 секунды параллельный
сервис превратился в последовательный. Сетевые вызовы под замком делать нельзя (кроме кейса 7).

---

## Кейс 5. Заказ из map — это КОПИЯ

Самый неочевидный кейс. Смотри на тип хранилища ([handler.go:61](pkg/handler/handler.go#L61)):
`orders map[uuid.UUID]Order` — значения, **не указатели**.

**Файл:** [handler.go:263](pkg/handler/handler.go#L263), в `PayOrder`

**БЫЛО:**
```go
order.Status = OrderStatusPaid
h.store.orders[params.OrderUUID] = order   // ← удали эту строку
```

**СТАЛО:**
```go
order.Status = OrderStatusPaid
```

**ЗАПУСТИ** — тест `TestDemoPayThenGet` (создать → оплатить → прочитать обратно)
уже готов в [tests/demo_test.go](tests/demo_test.go):
```bash
cd project/order
go test -tags demo ./tests/ -run TestDemoPayThenGet -count=1 -v
```

**УВИДИШЬ** — сравни два настоящих прогона:

```
СО строкой (как сейчас в коде):
  оплата: HTTP 200, transaction_uuid=&{e962e07f-...}
  GET после оплаты: HTTP 200 status="PAID" transaction_uuid=0x5c14294901b0

БЕЗ строки:
  оплата: HTTP 200, transaction_uuid=&{d2a71bde-...}
  GET после оплаты: HTTP 200 status="PENDING_PAYMENT" transaction_uuid=<nil>
                                       ↑ деньги списаны, а заказ не оплачен
--- PASS                               ↑ и тест при этом ЗЕЛЁНЫЙ
```

**ПОЧЕМУ:** `order, ok := h.store.orders[uuid]` достаёт **копию** структуры. Строки 260–262
меняют копию, лежащую на стеке, а в хранилище всё осталось как было. Запись обратно —
обязательна, и компилятор об этом не предупредит.

Почему тогда не хранить `map[uuid.UUID]*Order`? Тогда замок защищал бы только саму map,
а поля `Order` можно было бы менять по указателю уже вне замка — и `RLock` в `GetOrder`
от этой гонки бы не спас. Хранение по значению делает такую гонку невозможной по построению.

**ВЕРНИ:** строку 263 на место.

---

## Кейс 6. Дедлок: `Lock` дважды

**СТАЛО:**
```go
h.store.mu.Lock()
h.store.mu.Lock()   // ← та же горутина, тот же замок
h.store.orders[order.OrderUUID] = order
h.store.mu.Unlock()
```

**ЗАПУСТИ:** любой POST-запрос.

**УВИДИШЬ:** запрос виснет навсегда, curl отваливается по таймауту. В тесте (где других
активных горутин нет) рантайм добавит:
```
fatal error: all goroutines are asleep - deadlock!
```

**ПОЧЕМУ:** `sync.Mutex` в Go **не реентрантный** — в отличие от локов в Java/C#. Вторая
`Lock()` видит занятый замок и паркует горутину в ожидании, что кто-то отпустит. Отпустить
может только эта же горутина, которая теперь спит вечно.

В живом сервере сообщения о дедлоке **не будет** — там всегда есть другие живые горутины,
и рантайм считает, что всё в порядке. Сервис просто тихо перестанет отвечать: запросы один
за другим встанут в очередь за замком, который никто не отпустит. Диагностика в проде —
`kill -QUIT <pid>`: Go напечатает стеки всех горутин, и будет видна пачка застрявших
на `sync.(*RWMutex).Lock`.

**Практический вывод:** метод, который держит замок, не должен звать другой метод, который
берёт тот же замок. Обычное решение — публичный метод берёт замок и зовёт приватный
(`...Locked`), который считает, что замок уже взят.

---

## Кейс 7. Почему в `PayOrder` замок на весь метод

**Файл:** [handler.go:221-222](pkg/handler/handler.go#L221-L222)

**БЫЛО:**
```go
h.store.mu.Lock()
defer h.store.mu.Unlock()
```

**СТАЛО** (соблазнительная «оптимизация» — не держать замок во время сетевого вызова):
```go
h.store.mu.RLock()
order, ok := h.store.orders[params.OrderUUID]
h.store.mu.RUnlock()
// ...проверки статуса и вызов PaymentService без замка...
// а в конце:
h.store.mu.Lock()
h.store.orders[params.OrderUUID] = order
h.store.mu.Unlock()
```

**ЗАПУСТИ** — тест `TestDemoDoublePay` (две параллельные оплаты одного заказа)
уже готов в [tests/demo_test.go](tests/demo_test.go):
```bash
cd project/order
go test -tags demo ./tests/ -run TestDemoDoublePay -count=1 -v
```

**УВИДИШЬ:** оба запроса вернут **200** и оба спишут деньги. С текущим кодом (единый `Lock`)
второй получит **409 Conflict**.

**ПОЧЕМУ:** это гонка **check-then-write**: обе горутины успели прочитать `PENDING_PAYMENT`
до того, как первая записала `PAID`. Схема по шагам нарисована в
[HOW_IT_WORKS.md](HOW_IT_WORKS.md), часть 7.2.

Отсюда — почему в `CreateOrder` замок на 3 строки, а в `PayOrder` на весь метод:

| | `CreateOrder` | `PayOrder` |
|---|---|---|
| что делает | одна изолированная запись | прочитал → проверил → оплатил → записал |
| ключ | только что создан `uuid.New()`, конкурентов нет | существующий, за него могут бороться |
| замок | 3 строки, без `defer` | весь метод, с `defer` |
| почему так | дробить безопасно | дробить нельзя — двойная оплата |

`defer` в `PayOrder` не для красоты: между `Lock` и `Unlock` там **шесть разных `return`**,
и любой забытый `Unlock` навсегда заклинил бы сервис (кейс 6). В `CreateOrder` между
`Lock` и `Unlock` нет ни одного `return` — терять нечего.

---

## Шпаргалка

- Читаю → `RLock`/`RUnlock`. Пишу → `Lock`/`Unlock`. Читаю-проверяю-пишу → **один** `Lock` на всё.
- Несколько `return` внутри секции → `defer mu.Unlock()`.
- Секция максимально короткая; сетевые вызовы под замком — только когда без этого гонка.
- Замок не реентрантный: под замком не звать метод, который берёт тот же замок.
- `map[K]Order` (значения) — правку копии надо записывать обратно; `map[K]*Order` — открывает
  гонку по полям, которую мьютекс на map не ловит.
- `sync.Mutex` нельзя копировать — поэтому `store` везде передаётся как `*orderStore`. Ловит `go vet`.
- Проверять всё это только под `-race`.
