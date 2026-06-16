# Неделя 1 — HTTP и gRPC. Разбор и пошаговый план

> Это твой конспект-ментор по первой неделе курса «Микросервисы на Go 3.0».
> Здесь: что вообще происходит, как устроены файлы, что уже готово, что писать руками
> и в каком порядке. Идём строго последовательно.

---

## 0. Что мы вообще строим (картина целиком)

Мы делаем систему заказов на постройку космических кораблей из трёх **микросервисов**.
Микросервис — это отдельная маленькая программа, которая делает одну задачу и общается
с другими по сети.

```
            HTTP (JSON)                gRPC
  Клиент  ───────────────►  Order   ──────────►  Inventory  (:50051)
 (curl,                    Service  ──────────►  Payment    (:50052)
  Postman)                 (:8080)      gRPC
```

| Сервис        | Протокол | Порт  | Роль                                                        |
|---------------|----------|-------|-------------------------------------------------------------|
| **Order**     | HTTP     | 8080  | Принимает запросы от пользователя, оркеструет два других    |
| **Inventory** | gRPC     | 50051 | Каталог деталей (корпус, двигатель, щит, оружие)            |
| **Payment**   | gRPC     | 50052 | Проводит «оплату», возвращает transaction_uuid             |

**Главная идея недели:** научиться связывать HTTP-сервис с gRPC-сервисами.
Order принимает HTTP-запрос → внутри вызывает Inventory и Payment по gRPC → отдаёт HTTP-ответ.

### Два протокола — в чём разница

- **HTTP + JSON** — то, что ты видишь в обычном вебе. Текстовый формат, человекочитаемый,
  запрос вида `POST /api/v1/orders` с JSON-телом. Удобен «наружу», для клиентов.
- **gRPC** — бинарный протокол от Google поверх HTTP/2. Быстрый, строго типизированный.
  Контракт описывается в `.proto`-файле, по которому **генерируется** Go-код
  (структуры и клиент/сервер). Удобен «внутри» между сервисами.

### Космический корабль = набор слотов

Корабль состоит из именованных слотов, по одному компоненту в каждом:

| Слот     | Компонент      | Обязательный |
|----------|----------------|--------------|
| `hull`   | Корпус         | ✅ Да        |
| `engine` | Двигатель      | ✅ Да        |
| `shield` | Щит            | ❌ Нет       |
| `weapon` | Вооружение     | ❌ Нет       |

---

## 1. Главные правила сдачи (читать обязательно)

1. **Сдаётся в ДВУХ PR (pull request):**
   - **PR #1 — контракты:** только `.proto` + OpenAPI + сгенерированный код. Без реализации.
   - **PR #2 — реализация:** код всех трёх сервисов.
   - Один PR на всё = не принято.

2. **Весь пользовательский текст — на русском:** тексты ошибок, логи (`slog.Info("...")`),
   комментарии. На английском остаются только технические идентификаторы (имена функций,
   переменных, пакетов, JSON-теги, HTTP-пути).

3. **Версия Go — 1.26** (в `go.work` и во всех `go.mod`).
   ⚠️ **У тебя сейчас установлена Go 1.23.4** — это надо исправить (см. Шаг 1).

4. **Никаких `// TODO:` в финальном коде** — линтер настроен их ловить и будет ругаться.

5. **Перед каждым PR прогоняем:**
   ```bash
   task format     # форматирование
   task lint       # линтер без ошибок
   task test:api   # API-тесты (только для PR #2)
   ```
   В проекте настроен GitHub Actions CI — он проверяет то же самое и должен быть зелёным.

---

## 2. Ограничения недели (что НЕЛЬЗЯ делать)

Это важно: на первой неделе мы намеренно пишем «грязно и просто», чтобы
сфокусироваться на связке HTTP↔gRPC. Архитектура придёт на неделе 2.

❌ **Не** разделять код на слои (`api/`, `service/`, `repository/`) — вся логика в одном пакете.
❌ **Не** использовать dependency injection — клиенты создаём прямо в `main`.
❌ **Не** подключать БД — храним в `map` в памяти.
❌ **Не** добавлять метрики/трейсы (это неделя 7), конфиги/env (хардкодим порты).
❌ **Не** делать резервирование деталей — просто проверяем `stock_quantity > 0`.
❌ **Не** делать идемпотентность, авторизацию (неделя 6), проверку совместимости деталей.
❌ **Не** оборачивать ошибки `fmt.Errorf("...: %w", err)` — это неделя 2.

✅ Логирование — через `slog` из стандартной библиотеки.
✅ gRPC-ошибки — через `status.Error()` / `status.Errorf()` с нужным кодом.
✅ HTTP-сервис маппит gRPC-коды в HTTP-статусы через `status.FromError()`.

---

## 3. Как устроен проект (объяснение инструментов)

Твоя папка `project/` — это уже скопированный boilerplate (заготовка). Разберём ключевые штуки.

### `go.work` — Go workspace
Объединяет несколько Go-модулей (order, inventory, payment, shared) в одно рабочее
пространство, чтобы они видели друг друга локально без публикации. Внутри:
```
use (
    ./inventory
    ./order
    ./payment
    ./shared
)
```

### Четыре модуля (каждый со своим `go.mod`)
- **`shared`** — общий код: сюда **генерируется** Go-код из proto и OpenAPI.
  Здесь же лежат сами контракты (`proto/` и `api/`).
- **`inventory`** — gRPC-сервер каталога деталей.
- **`payment`** — gRPC-сервер оплаты.
- **`order`** — HTTP-сервер + gRPC-клиенты к двум другим.

### `shared/proto/` — gRPC-контракты
`.proto`-файлы. Из них командой `task proto:gen` генерируется Go-код
(структуры сообщений + интерфейсы клиента и сервера) в `shared/pkg/proto/`.
Используется инструмент **buf** (обёртка над protoc).

### `shared/api/` — OpenAPI-контракт (для HTTP)
YAML-файлы, описывающие REST API сервиса Order. Из них командой `task ogen:gen`
инструмент **ogen** генерирует Go-код HTTP-сервера в `shared/pkg/openapi/`.
Этот контракт **уже полностью готов** — трогать не надо, только изучить.

### `Taskfile.yaml` — запуск команд (как Makefile)
Инструмент **task**. Полезные команды:
```bash
task setup       # установить все инструменты (buf, ogen, линтеры) в ./bin
task proto:gen   # сгенерировать Go-код из .proto
task ogen:gen    # сгенерировать Go-код из OpenAPI
task format      # отформатировать код
task lint        # запустить линтер
task test:api    # запустить API-тесты
```

### `.golangci.yml` — конфиг линтера
Набор правил проверки кода. Без этого файла `task lint` падает с `can't load config`.

### `scripts/pre-commit` — git-хук
Автоформат + линтер перед каждым коммитом. Подключается **опционально** и
**только после PR #1** (пока в коде есть TODO — хук не даст закоммитить).

---

## 4. Что УЖЕ готово, а что писать руками

### ✅ Готово в заготовке
- OpenAPI-спецификация (`shared/api/order/v1/`) — полностью.
- `order/pkg/handler/handler.go` — структуры `Order`, `OrderStore`, метод `GetOrder`
  как пример. Остальные методы — TODO.
- `inventory/pkg/service/service.go` — структура `Part` и **seed-данные (7 деталей)** готовы.
  Методы `GetPart`/`ListParts` — TODO.
- `payment/pkg/service/service.go` — каркас. Метод `PayOrder` — TODO.
- `cmd/main.go` всех трёх сервисов — каркасы с TODO (keepalive, graceful shutdown).
- Тесты `order/tests/api_test.go` — готовы, их менять не надо, они проверяют твою работу.

### ✍️ Писать руками
**PR #1 (контракты):**
- Дописать `shared/proto/inventory/v1/inventory.proto` по TODO.
- Дописать `shared/proto/payment/v1/payment.proto` по TODO.
- Заменить `github.com/student` на свой модуль везде.
- Сгенерировать код (`task proto:gen`, `task ogen:gen`).

**PR #2 (реализация):**
- `inventory`: `GetPart`, `ListParts`.
- `payment`: `PayOrder`.
- `order`: `CreateOrder`, `PayOrder`, `CancelOrder` + допилить `main.go`
  (keepalive, HTTP-таймауты, graceful shutdown, второй gRPC-клиент).

---

## 5. Контракты — точные спецификации

### 5.1. Inventory (gRPC, :50051)

**enum PartType:**
```
PART_TYPE_UNSPECIFIED = 0
PART_TYPE_HULL        = 1
PART_TYPE_ENGINE      = 2
PART_TYPE_SHIELD      = 3
PART_TYPE_WEAPON      = 4
```

**message Part:** `uuid` (string), `name` (string), `description` (string),
`price` (int64, копейки), `part_type` (PartType), `stock_quantity` (int64),
`created_at` (google.protobuf.Timestamp).

**Методы:**
- `GetPart(uuid)` → `Part`.
  - пустой/невалидный uuid → `INVALID_ARGUMENT`
  - не найдена → `NOT_FOUND`
- `ListParts(part_type, uuids[])` → `parts[]`.
  - если `uuids` не пуст → ищем именно их (порядок как в запросе), `part_type` игнор;
    любой невалидный → `INVALID_ARGUMENT`, любой не найден → `NOT_FOUND`.
  - если `uuids` пуст → фильтр по типу (`UNSPECIFIED` = все), результат сортируется по имени.

**Seed-данные (уже в заготовке):**
| UUID (…0001-0007) | Name               | Type   | Price   | Stock |
|-------------------|--------------------|--------|---------|-------|
| 0001              | Алюминиевый корпус | HULL   | 500000  | 10    |
| 0002              | Титановый корпус   | HULL   | 1500000 | 5     |
| 0003              | Ионный двигатель C | ENGINE | 300000  | 8     |
| 0004              | Ионный двигатель B | ENGINE | 800000  | 3     |
| 0005              | Энергетический щит | SHIELD | 400000  | 6     |
| 0006              | Лазерная пушка     | WEAPON | 250000  | 7     |
| 0007              | Плазменный корпус  | HULL   | 2000000 | 0 ⚠️  |

### 5.2. Payment (gRPC, :50052)

**enum PaymentMethod:**
```
PAYMENT_METHOD_UNSPECIFIED    = 0
PAYMENT_METHOD_CARD           = 1
PAYMENT_METHOD_SBP            = 2
PAYMENT_METHOD_CREDIT_CARD    = 3
PAYMENT_METHOD_INVESTOR_MONEY = 4
```

**Метод `PayOrder(order_uuid, payment_method)` → `transaction_uuid`:**
- пустой/невалидный `order_uuid` → `INVALID_ARGUMENT`
- `payment_method == UNSPECIFIED` → `INVALID_ARGUMENT`
- иначе: генерируем `transaction_uuid` (UUID v4), логируем через `slog`, возвращаем.
- Сервис **stateless** — ничего не хранит, всегда успешно.

### 5.3. Order (HTTP, :8080)

Префикс всех путей: `/api/v1`.

| Метод+путь                          | Успех | Ошибки                                   |
|-------------------------------------|-------|------------------------------------------|
| `POST /orders`                      | 201   | 400 (нет hull/engine, плохой UUID), 404 (деталь не найдена), 409 (нет на складе) |
| `GET /orders/{uuid}`                | 200   | 400 (плохой UUID), 404 (заказ не найден) |
| `POST /orders/{uuid}/pay`           | 200   | 400, 404, 409 (уже оплачен/отменён)      |
| `POST /orders/{uuid}/cancel`        | 200   | 400, 404, 409 (уже оплачен)              |

**OrderStatus:** `PENDING_PAYMENT` → `PAID` или `CANCELLED`.

**Поведение CreateOrder:**
1. `hull_uuid` и `engine_uuid` обязательны (иначе 400); все UUID валидного формата (иначе 400).
2. Собрать все переданные UUID в один список и вызвать `Inventory.ListParts(uuids)`.
3. Все детали существуют (иначе 404), у каждой `stock_quantity > 0` (иначе 409).
4. `total_price` = сумма цен; сгенерировать `order_uuid`; статус `PENDING_PAYMENT`; сохранить в map.

**Поведение PayOrder:** найти заказ (404) → статус == `PENDING_PAYMENT` (иначе 409) →
вызвать `Payment.PayOrder` → сохранить `transaction_uuid`, `payment_method`, статус → `PAID`.

**Поведение CancelOrder:** найти заказ (404) → `PENDING_PAYMENT` можно отменить;
`PAID`/`CANCELLED` → 409 → статус `CANCELLED`.

**⚠️ Маппинг PaymentMethod:** в HTTP короткие значения (`CARD`, `SBP`, …),
в gRPC с префиксом (`PAYMENT_METHOD_CARD`, …). При вызове Payment надо конвертировать.

**Обработка gRPC-ошибок в HTTP-хендлере:**
```go
st, ok := status.FromError(err)
if ok {
    switch st.Code() {
    case codes.NotFound:        // → HTTP 404
    case codes.InvalidArgument: // → HTTP 400
    default:                    // → HTTP 500
    }
}
```

---

## 6. Полезные технические подсказки

**Keepalive для gRPC** (вынести в именованные константы):
```go
const (
    grpcMaxConnectionIdle     = 15 * time.Minute
    grpcMaxConnectionAge      = 30 * time.Minute
    grpcMaxConnectionAgeGrace = 5  * time.Second
    grpcKeepaliveTime         = 5  * time.Minute
    grpcKeepaliveTimeout      = 1  * time.Second
    grpcMinPingInterval       = 5  * time.Minute
)
```
Сервер: `grpc.KeepaliveParams` + `grpc.KeepaliveEnforcementPolicy`.
Клиент: `grpc.WithKeepaliveParams`.

**Таймауты на gRPC-вызовы** из HTTP-хендлера:
```go
ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
defer cancel()
```

**Хранилище заказов:** `map[uuid.UUID]Order` под `sync.RWMutex` (защита от гонок).
**UUID:** генерация и валидация — `github.com/google/uuid`.

---

## 7. Пошаговый план (трекер 12 пунктов)

### PR #1 — Контракты
- [ ] **1. Скопировать boilerplate** — ✅ уже сделано (папка `project/` готова).
- [ ] **2. Дописать `inventory.proto`** (метод ListParts, типы деталей, поля Part, Response-сообщения).
- [ ] **3. Дописать `payment.proto`** (способы оплаты, поля запроса, Response-сообщение).
- [ ] **4. Заменить `github.com/student`** на свой модуль во всех файлах.
- [ ] **5. `task proto:gen` и `task ogen:gen` без ошибок.**
- [ ] **6. Создать PR с контрактами.**

### PR #2 — Реализация
- [ ] **7. Реализовать InventoryService** (`GetPart`, `ListParts`).
- [ ] **8. Реализовать PaymentService** (`PayOrder`).
- [ ] **9. Реализовать OrderService** (`CreateOrder`, `PayOrder`, `CancelOrder`, + `main.go`).
- [ ] **10. `task lint` и `task test:api` без ошибок.**
- [ ] **11. Настроить git-хук** (`scripts/pre-commit`) — опционально.
- [ ] **12. Создать PR с реализацией, CI зелёный.**

---

## 8. С чего начинаем прямо сейчас (Шаг 1)

Перед написанием контрактов нужно подготовить окружение:

1. **Поставить Go 1.26** (сейчас 1.23.4). Варианты: скачать с go.dev или `brew install go`.
2. **Определиться с именем модуля.** Сейчас везде `github.com/student`.
   Замени на свой, например `github.com/<твой-github-ник>/<имя-репо>`.
   Менять надо в: всех `go.mod`, `go.work`-комментарии не критичны, `Taskfile.yaml`
   (prefix в gci), `.golangci.yml` (секция gci), `go_package` в обоих `.proto`, и в импортах кода.
3. **Инициализировать git-репозиторий** (если ещё нет) — задание сдаётся через PR на GitHub.
4. **`task setup`** — установить инструменты (buf, ogen, линтеры) в `./bin`.

После этого переходим к пунктам 2-3 (дописываем proto) — я буду объяснять каждое поле.

---

## 9. Тесты — как проверяется работа

`order/tests/api_test.go` поднимает все три сервиса **в памяти**: gRPC через `bufconn`
(без реальных портов), HTTP через `httptest` — всё в одном процессе. Запуск: `task test:api`.

Проверяются happy-path (создать→оплатить, создать→отменить, опциональные компоненты)
и corner-cases (плохой UUID→400, нет заказа→404, нет компонента→404, повторная оплата→409,
отмена оплаченного→409, нет обязательных полей→400, плохой payment_method→400).

Менять тесты не нужно — наоборот, они эталон того, как должен вести себя твой код.

---

> Готов? Скажи — и начнём с **Шага 1** (окружение), а дальше пойдём по proto-файлам поле за полем.
