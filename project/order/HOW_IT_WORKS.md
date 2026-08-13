# Order Service — как всё работает, от начала до конца

Документ для того, кто только знакомится с Go и с кодогенерацией.
Главный вопрос, на который он отвечает: **что здесь написано руками, что сгенерировано,
и на основании чего**.

---

## Часть 0. Три категории файлов

В проекте каждый файл относится к одной из трёх групп. Пока не разложишь их по полкам —
будет ощущение, что код появляется из ниоткуда.

```
┌─────────────────────────────────────────────────────────────────────────┐
│  1. КОНТРАКТЫ — пишем руками, это описание API на языке разметки        │
│     shared/api/order/v1/*.yaml      (OpenAPI — описание HTTP API)      │
│     shared/proto/**/*.proto         (Protobuf — описание gRPC API)     │
│     Это НЕ Go-код. Это текстовые файлы-описания.                        │
└───────────────────────────┬─────────────────────────────────────────────┘
                            │  запускаем команду генерации
                            ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  2. СГЕНЕРИРОВАННЫЙ КОД — Go-файлы, созданные программой из контрактов  │
│     shared/pkg/openapi/order/v1/*.go   (создал ogen — 5713 строк)      │
│     shared/pkg/proto/**/*.go           (создал buf)                    │
│     Руками НЕ трогаем. При следующей генерации перезапишется.           │
└───────────────────────────┬─────────────────────────────────────────────┘
                            │  используем в своём коде
                            ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  3. НАШ КОД — бизнес-логика, пишем руками                              │
│     order/pkg/handler/handler.go   (~290 строк)                        │
│     order/cmd/main.go              (~125 строк)                        │
└─────────────────────────────────────────────────────────────────────────┘
```

Соотношение объёмов: контракт на 22 небольших `.yaml`-файла → **5713 строк** Go-кода →
наши **415 строк** логики. Кодогенерация избавляет от написания рутины: разбор JSON,
маршрутизация, валидация, сериализация ответов.

---

## Часть 1. Контракт — что это и как читается

### Точка входа: `order.openapi.yaml`

Файл: [shared/api/order/v1/order.openapi.yaml](../shared/api/order/v1/order.openapi.yaml)

```yaml
openapi: 3.0.3                       # версия стандарта OpenAPI
info:
  title: Order Service API
  version: 1.0.0

paths:                               # какие URL поддерживает сервис
  /api/v1/orders:
    $ref: ./paths/orders.yaml        # ← ссылка на другой файл
  /api/v1/orders/{order_uuid}:
    $ref: ./paths/order_by_uuid.yaml
  /api/v1/orders/{order_uuid}/pay:
    $ref: ./paths/order_pay.yaml
  /api/v1/orders/{order_uuid}/cancel:
    $ref: ./paths/order_cancel.yaml
```

**`$ref`** — механизм OpenAPI для разбиения спецификации на файлы. Вместо одной
огромной портянки — 22 маленьких файла. Генератор при чтении подставляет содержимое
по ссылкам, как если бы всё было написано в одном месте.

**`{order_uuid}` в фигурных скобках** — параметр пути. Означает «сюда подставится любое
значение», а конкретное значение будет доступно в коде.

### Описание одной операции

Файл: [shared/api/order/v1/paths/orders.yaml](../shared/api/order/v1/paths/orders.yaml)

```yaml
post:                                # HTTP-метод
  summary: Создать заказ
  operationId: createOrder           # ← из этого имени родится метод Go: CreateOrder
  requestBody:
    required: true                   # тело обязательно
    content:
      application/json:              # только JSON
        schema:
          $ref: '../components/create_order_request.yaml'
  responses:
    '201':                           # ← из каждого кода родится отдельный Go-тип
      description: Заказ успешно создан
      content:
        application/json:
          schema:
            $ref: '../components/create_order_response.yaml'
    '400': { $ref: '../components/responses/bad_request.yaml' }
    '404': { $ref: '../components/responses/component_not_found.yaml' }
    '409': { $ref: '../components/responses/component_out_of_stock.yaml' }
    '500': { $ref: '../components/responses/internal_server_error.yaml' }
```

Запомни `operationId: createOrder` и список кодов `201/400/404/409/500` — сейчас увидим,
во что именно они превращаются.

### Описание структуры данных

Файл: [shared/api/order/v1/components/create_order_request.yaml](../shared/api/order/v1/components/create_order_request.yaml)

```yaml
type: object
required:                            # ← эти поля обязательны
  - hull_uuid
  - engine_uuid
properties:
  hull_uuid:
    type: string
    format: uuid                     # ← не просто строка, а UUID
    description: UUID корпуса (обязательный, v4)
  engine_uuid:
    type: string
    format: uuid
  shield_uuid:
    type: string
    format: uuid
    nullable: true                   # ← может быть null; в required не указан → опционально
  weapon_uuid:
    type: string
    format: uuid
    nullable: true
```

Три вещи, которые здесь заданы и позже станут кодом:
1. **имена полей** (`hull_uuid`) → станут полями Go-структуры
2. **`format: uuid`** → породит проверку формата и тип `uuid.UUID` вместо `string`
3. **`required` / `nullable`** → определят, будет поле обычным или в обёртке для опциональных

---

## Часть 2. Команда генерации — что происходит

### Сама команда

Определена в [Taskfile.yaml:120-126](../Taskfile.yaml):

```yaml
ogen:gen:
  desc: "Генерирует Go-код из OpenAPI"
  deps: [ogen:install]               # сначала убедиться, что ogen установлен
  dir: shared                        # выполнять из папки shared/
  cmds:
    - '{{.OGEN}} -target ./pkg/openapi/order/v1 -package orderv1 -clean ./api/order/v1/order.openapi.yaml'
```

Разбор флагов:

| Часть | Значение |
|-------|----------|
| `{{.OGEN}}` | путь к программе: `./bin/ogen` (версия `v1.20.3`, см. [Taskfile.yaml:12](../Taskfile.yaml)) |
| `-target ./pkg/openapi/order/v1` | **куда** класть сгенерированные файлы |
| `-package orderv1` | **имя Go-пакета** в сгенерированных файлах |
| `-clean` | сначала удалить старые файлы в target (чтобы не осталось мусора) |
| `./api/order/v1/order.openapi.yaml` | **что** читать — точка входа спецификации |

Откуда взялась программа `./bin/ogen`: её поставила команда `task setup` (или `ogen:install`),
которая выполнила `go install github.com/ogen-go/ogen/cmd/ogen@v1.20.3` — скачала исходники
из интернета, собрала и положила бинарник в `./bin/`.

### Что происходит при запуске `task ogen:gen`

```
1. ogen читает order.openapi.yaml
2. Рекурсивно подставляет всё по $ref — получает единое дерево описания API
3. Проверяет, что спецификация валидна (иначе падает с ошибкой)
4. Обходит дерево и для каждого элемента пишет Go-код по своим шаблонам
5. Форматирует результат gofmt-ом и раскладывает по 18 файлам
```

Результат — папка `shared/pkg/openapi/order/v1/`, 18 файлов, 5713 строк:

| Файл | Строк | Что внутри |
|------|-------|-----------|
| `oas_json_gen.go` | 1625 | сериализация/десериализация JSON для каждой структуры |
| `oas_response_decoders_gen.go` | 712 | разбор ответов (нужно для клиента) |
| `oas_handlers_gen.go` | 623 | обвязка вокруг вызова **твоих** методов |
| `oas_schemas_gen.go` | 578 | **структуры данных** — то, с чем ты работаешь |
| `oas_client_gen.go` | 455 | HTTP-клиент (нам не нужен, но генерируется) |
| `oas_router_gen.go` | 406 | маршрутизация URL → операция |
| `oas_cfg_gen.go` | 318 | конфигурация сервера |
| `oas_response_encoders_gen.go` | 287 | **тип ответа → HTTP-статус** |
| `oas_parameters_gen.go` | 214 | разбор параметров пути |
| `oas_request_decoders_gen.go` | 165 | разбор тела запроса |
| `oas_validators_gen.go` | 100 | проверки значений |
| `oas_server_gen.go` | 56 | **интерфейс `Handler`** — то, что ты реализуешь |
| `oas_unimplemented_gen.go` | 52 | заглушка `UnimplementedHandler` |
| остальные 5 | ~120 | вспомогательное |

Первая строка каждого файла:

```go
// Code generated by ogen, DO NOT EDIT.
```

Это стандартная в Go пометка: файл машинный, правки будут стёрты.

---

## Часть 3. Карта генерации: какая строка контракта во что превратилась

Это навигационный раздел. Здесь для каждого файла контракта указано, **что именно**
из него родилось и **в какой строке какого файла** это лежит. Все ссылки кликабельны.

Общая механика: ogen читает `order.openapi.yaml`, разворачивает все `$ref` в одно дерево
и обходит его. Каждый узел дерева порождает свои Go-элементы по фиксированным правилам.

---

### 3.1. Файлы контракта и их «выхлоп» — сводная таблица

| Файл контракта | Строк | Что породил |
|----------------|-------|-------------|
| [order.openapi.yaml](../shared/api/order/v1/order.openapi.yaml) | 32 | таблицу маршрутов в [oas_router_gen.go](../shared/pkg/openapi/order/v1/oas_router_gen.go) |
| [paths/orders.yaml](../shared/api/order/v1/paths/orders.yaml) | 27 | метод `CreateOrder` + 5 типов ответа |
| [paths/order_by_uuid.yaml](../shared/api/order/v1/paths/order_by_uuid.yaml) | 22 | метод `GetOrder` + 4 типа ответа |
| [paths/order_pay.yaml](../shared/api/order/v1/paths/order_pay.yaml) | 29 | метод `PayOrder` + 5 типов ответа |
| [paths/order_cancel.yaml](../shared/api/order/v1/paths/order_cancel.yaml) | 23 | метод `CancelOrder` + 5 типов ответа |
| [params/order_uuid.yaml](../shared/api/order/v1/params/order_uuid.yaml) | 8 | 3 структуры `*Params` + 3 функции разбора |
| [components/create_order_request.yaml](../shared/api/order/v1/components/create_order_request.yaml) | 27 | `CreateOrderRequest` + геттеры/сеттеры |
| [components/create_order_response.yaml](../shared/api/order/v1/components/create_order_response.yaml) | 15 | `CreateOrderResponse` |
| [components/order_dto.yaml](../shared/api/order/v1/components/order_dto.yaml) | 59 | `OrderDto` (10 полей) + валидатор |
| [components/pay_order_request.yaml](../shared/api/order/v1/components/pay_order_request.yaml) | 6 | `PayOrderRequest` + валидатор |
| [components/pay_order_response.yaml](../shared/api/order/v1/components/pay_order_response.yaml) | 9 | `PayOrderResponse` |
| [components/cancel_order_response.yaml](../shared/api/order/v1/components/cancel_order_response.yaml) | 2 | `CancelOrderResponse` (пустая структура) |
| [components/errors/error.yaml](../shared/api/order/v1/components/errors/error.yaml) | 25 | `Error` + **15** типов-псевдонимов ошибок |
| [components/enums/order_status.yaml](../shared/api/order/v1/components/enums/order_status.yaml) | 7 | `OrderStatus` + 3 константы + валидатор |
| [components/enums/payment_method.yaml](../shared/api/order/v1/components/enums/payment_method.yaml) | 8 | `PaymentMethod` + 4 константы + валидатор |
| `components/responses/*.yaml` (6 файлов) | ~8 каждый | привязку HTTP-кода к типу в энкодере |

Итого 22 файла контракта (~330 строк YAML) → 18 Go-файлов (5713 строк).

---

### 3.2. `paths/orders.yaml` → операция createOrder

Разбор построчно. Слева фрагмент контракта, справа — куда он попал.

```yaml
 1  post:                                    → HTTP-метод в роутере
 4    operationId: createOrder               → имя Go-метода CreateOrder
 7    requestBody:                           → появляется аргумент req
12          $ref: create_order_request.yaml  → тип этого аргумента
14      '201':                               → тип успешного ответа
19            $ref: create_order_response.yaml
20      '400': …bad_request.yaml             → тип CreateOrderBadRequest
22      '404': …component_not_found.yaml     → тип CreateOrderNotFound
24      '409': …component_out_of_stock.yaml  → тип CreateOrderConflict
26      '500': …internal_server_error.yaml   → тип CreateOrderInternalServerError
```

Точные адреса порождённого:

| Из контракта | Что породило | Где лежит |
|--------------|--------------|-----------|
| `operationId: createOrder` (стр. 4) | метод интерфейса | [oas_server_gen.go:23](../shared/pkg/openapi/order/v1/oas_server_gen.go#L23) |
| | заглушка | [oas_unimplemented_gen.go:31](../shared/pkg/openapi/order/v1/oas_unimplemented_gen.go#L31) |
| | обвязка вызова | [oas_handlers_gen.go:185](../shared/pkg/openapi/order/v1/oas_handlers_gen.go#L185) |
| | константа имени | [oas_operations_gen.go](../shared/pkg/openapi/order/v1/oas_operations_gen.go) |
| | интерфейс-маркер `CreateOrderRes` | [oas_interfaces_gen.go:8](../shared/pkg/openapi/order/v1/oas_interfaces_gen.go#L8) |
| `requestBody` (стр. 7-12) | разбор тела | [oas_request_decoders_gen.go:17](../shared/pkg/openapi/order/v1/oas_request_decoders_gen.go#L17) |
| `'201'` (стр. 14-19) | `CreateOrderResponse` | [oas_schemas_gen.go:104](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L104) |
| `'400'` (стр. 20-21) | `CreateOrderBadRequest` | [oas_schemas_gen.go:35](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L35) |
| `'404'` (стр. 22-23) | `CreateOrderNotFound` | [oas_schemas_gen.go:47](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L47) |
| `'409'` (стр. 24-25) | `CreateOrderConflict` | [oas_schemas_gen.go:39](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L39) |
| `'500'` (стр. 26-27) | `CreateOrderInternalServerError` | [oas_schemas_gen.go:43](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L43) |
| весь блок `responses` | энкодер тип→статус | [oas_response_encoders_gen.go:86](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L86) |

**Наша реализация:** [handler.go:150](pkg/handler/handler.go#L150)

---

### 3.3. Где именно назначается HTTP-статус

Самое неочевидное место. В контракте написано `'404':` — а превращается это вот во что
([oas_response_encoders_gen.go:114-116](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L114)):

```go
case *CreateOrderNotFound:
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(404)
```

Полная таблица «тип ответа → строка, где пишется статус»:

| Операция | Тип, который возвращаем | Статус | Строка энкодера |
|----------|-------------------------|--------|-----------------|
| **createOrder** | `*CreateOrderResponse` | 201 | [:90](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L90) |
| | `*CreateOrderBadRequest` | 400 | [:103](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L103) |
| | `*CreateOrderNotFound` | 404 | [:116](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L116) |
| | `*CreateOrderConflict` | 409 | [:129](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L129) |
| | `*CreateOrderInternalServerError` | 500 | [:142](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L142) |
| **getOrder** | `*OrderDto` | 200 | [:162](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L162) |
| | `*GetOrderBadRequest` | 400 | [:175](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L175) |
| | `*GetOrderNotFound` | 404 | [:188](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L188) |
| | `*GetOrderInternalServerError` | 500 | [:201](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L201) |
| **payOrder** | `*PayOrderResponse` | 200 | [:221](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L221) |
| | `*PayOrderBadRequest` | 400 | [:234](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L234) |
| | `*PayOrderNotFound` | 404 | [:247](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L247) |
| | `*PayOrderConflict` | 409 | [:260](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L260) |
| | `*PayOrderInternalServerError` | 500 | [:273](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L273) |
| **cancelOrder** | `*CancelOrderResponse` | 200 | [:18](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L18) |
| | `*CancelOrderBadRequest` | 400 | [:31](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L31) |
| | `*CancelOrderNotFound` | 404 | [:44](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L44) |
| | `*CancelOrderConflict` | 409 | [:57](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L57) |
| | `*CancelOrderInternalServerError` | 500 | [:70](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L70) |

**Проверь себя на исключении.** В таблице у `getOrder` нет строки с 409. Открой
[paths/order_by_uuid.yaml](../shared/api/order/v1/paths/order_by_uuid.yaml) — там
в `responses` перечислены только `200/400/404/500`, кода `409` нет. Значит, типа
`GetOrderConflict` **не существует**, и попытка его вернуть не скомпилируется.
Контракт напрямую ограничивает, что метод физически может вернуть.

---

### 3.4. Три остальные операции — те же адреса

| | createOrder | getOrder | payOrder | cancelOrder |
|--|-------------|----------|----------|-------------|
| Контракт | [orders.yaml](../shared/api/order/v1/paths/orders.yaml) | [order_by_uuid.yaml](../shared/api/order/v1/paths/order_by_uuid.yaml) | [order_pay.yaml](../shared/api/order/v1/paths/order_pay.yaml) | [order_cancel.yaml](../shared/api/order/v1/paths/order_cancel.yaml) |
| Метод в `Handler` | [:23](../shared/pkg/openapi/order/v1/oas_server_gen.go#L23) | [:30](../shared/pkg/openapi/order/v1/oas_server_gen.go#L30) | [:36](../shared/pkg/openapi/order/v1/oas_server_gen.go#L36) | [:16](../shared/pkg/openapi/order/v1/oas_server_gen.go#L16) |
| Обвязка вызова | [:185](../shared/pkg/openapi/order/v1/oas_handlers_gen.go#L185) | [:329](../shared/pkg/openapi/order/v1/oas_handlers_gen.go#L329) | [:472](../shared/pkg/openapi/order/v1/oas_handlers_gen.go#L472) | [:41](../shared/pkg/openapi/order/v1/oas_handlers_gen.go#L41) |
| Разбор тела | [:17](../shared/pkg/openapi/order/v1/oas_request_decoders_gen.go#L17) | — | [:88](../shared/pkg/openapi/order/v1/oas_request_decoders_gen.go#L88) | — |
| Структура `Params` | — | [:85](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L85) | [:151](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L151) | [:19](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L19) |
| Разбор `Params` | — | [:101](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L101) | [:167](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L167) | [:35](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L35) |
| Энкодер ответов | [:86](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L86) | [:158](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L158) | [:217](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L217) | [:14](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go#L14) |
| **Наш код** | [handler.go:150](pkg/handler/handler.go#L150) | [handler.go:99](pkg/handler/handler.go#L99) | [handler.go:215](pkg/handler/handler.go#L215) | [handler.go:270](pkg/handler/handler.go#L270) |

Обрати внимание на прочерки — они объясняют, почему у методов разные сигнатуры:

- у `createOrder` нет `Params`, потому что в пути `/api/v1/orders` нет `{параметров}`
- у `getOrder` и `cancelOrder` нет разбора тела, потому что в их контракте нет `requestBody`
- у `payOrder` есть и то и другое → в сигнатуре три аргумента

---

### 3.5. `create_order_request.yaml` → структура запроса

Контракт ([create_order_request.yaml](../shared/api/order/v1/components/create_order_request.yaml)):

```yaml
 1  type: object
 2  required:
 3    - hull_uuid            ← в required
 4    - engine_uuid          ← в required
 5  properties:
 6    hull_uuid:
 7      type: string
 8      format: uuid
 9      description: UUID корпуса (обязательный, v4)
16    shield_uuid:
17      type: string
18      format: uuid
19      nullable: true       ← НЕ в required + nullable
```

Результат ([oas_schemas_gen.go:52](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L52)):

```go
type CreateOrderRequest struct {
	// UUID корпуса (обязательный, v4).        ← из description, строка 9
	HullUUID   uuid.UUID  `json:"hull_uuid"`   ← из строк 3, 6, 8
	EngineUUID uuid.UUID  `json:"engine_uuid"`
	// UUID щита (опциональный, v4).
	ShieldUUID OptNilUUID `json:"shield_uuid"` ← из строк 16, 19 (nullable → Opt)
	WeaponUUID OptNilUUID `json:"weapon_uuid"`
}
```

Правила преобразования, каждое видно на этом примере:

| Что в YAML | Что в Go | Правило |
|------------|----------|---------|
| `hull_uuid` | `HullUUID` | snake_case → CamelCase |
| | `` `json:"hull_uuid"` `` | тег хранит исходное имя, чтобы JSON не изменился |
| `type: string` + `format: uuid` | `uuid.UUID` | формат меняет Go-тип |
| имя есть в `required` | `uuid.UUID` | обычное поле |
| имени нет в `required` + `nullable: true` | `OptNilUUID` | обёртка «может отсутствовать» |
| `description:` | комментарий | документация переезжает в код |

Плюс к структуре генерируются 8 методов доступа
([oas_schemas_gen.go:64-102](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L64)) —
по паре `GetXxx`/`SetXxx` на каждое поле.

**Где мы это используем:** [handler.go:151-161](pkg/handler/handler.go#L151)

---

### 3.6. `order_dto.yaml` → структура ответа (самая большая)

Контракт — 10 свойств, [order_dto.yaml](../shared/api/order/v1/components/order_dto.yaml).
Результат — [oas_schemas_gen.go:300](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L300):

| Строка YAML | Свойство | В `required`? | Поле Go | Почему такой тип |
|-------------|----------|---------------|---------|------------------|
| 10-14 | `order_uuid` | да (стр. 3) | `OrderUUID uuid.UUID` | обязательное + `format: uuid` |
| 15-19 | `hull_uuid` | да (стр. 4) | `HullUUID uuid.UUID` | то же |
| 20-24 | `engine_uuid` | да (стр. 5) | `EngineUUID uuid.UUID` | то же |
| 25-30 | `shield_uuid` | нет, `nullable` | `ShieldUUID OptNilUUID` | опциональное |
| 31-36 | `weapon_uuid` | нет, `nullable` | `WeaponUUID OptNilUUID` | опциональное |
| 37-41 | `total_price` | да (стр. 6) | `TotalPrice int64` | `type: integer` + `format: int64` |
| 42-47 | `transaction_uuid` | нет, `nullable` | `TransactionUUID OptNilUUID` | опциональное |
| 48-52 | `payment_method` | нет, `nullable` + `$ref` на enum | `PaymentMethod OptNilPaymentMethod` | опциональный enum |
| 53-54 | `status` | да (стр. 7), `$ref` на enum | `Status OrderStatus` | обязательный enum — **без обёртки** |
| 55-58 | `created_at` | да (стр. 8) | `CreatedAt time.Time` | `format: date-time` → `time.Time` |

Сравни строки `payment_method` и `status`: оба ссылаются на enum, но у первого
`nullable: true` и его нет в `required` → обёртка `OptNilPaymentMethod`;
второй в `required` → голый тип `OrderStatus`. Одно слово в контракте меняет тип в коде.

**Где мы это заполняем:** [handler.go:127-145](pkg/handler/handler.go#L127) — обратное
преобразование, из указателей нашей структуры `Order` в `OptNil…`-обёртки.

---

### 3.7. `error.yaml` → один тип и 15 псевдонимов

Контракт ([errors/error.yaml](../shared/api/order/v1/components/errors/error.yaml)):

```yaml
type: object
required: [code, message]
properties:
  code:    { type: integer }
  message: { type: string }
```

Породил базовую структуру [oas_schemas_gen.go:134](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L134):

```go
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
```

А дальше — **по одному псевдониму на каждую пару (операция, код ошибки)**. Всего 15
(не 16, потому что у `getOrder` кода 409 в контракте нет):

| | 400 | 404 | 409 | 500 |
|--|-----|-----|-----|-----|
| createOrder | [:35](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L35) | [:47](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L47) | [:39](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L39) | [:43](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L43) |
| getOrder | [:161](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L161) | [:169](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L169) | *нет в контракте* | [:165](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L165) |
| payOrder | [:474](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L474) | [:486](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L486) | [:478](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L478) | [:482](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L482) |
| cancelOrder | [:12](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L12) | [:24](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L24) | [:16](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L16) | [:20](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L20) |

Каждый объявлен так:

```go
type CreateOrderNotFound Error          // тот же набор полей, но ДРУГОЙ тип
func (*CreateOrderNotFound) createOrderRes() {}   // метод-маркер
```

**Зачем 15 типов вместо одного.** Метод-маркер `createOrderRes()` есть только у типов
операции `createOrder`. Интерфейс
[`CreateOrderRes`](../shared/pkg/openapi/order/v1/oas_interfaces_gen.go#L8) требует именно
этот метод. Значит, вернуть из `CreateOrder` тип `PayOrderNotFound` **не даст компилятор** —
у того другой маркер (`payOrderRes()`). Контракт превращается в проверку на этапе сборки.

Метод с маленькой буквы (`createOrderRes`, а не `CreateOrderRes`) в Go не виден за пределами
своего пакета. Поэтому в нашем пакете `handler` нельзя объявить свой тип с таким методом —
список допустимых ответов закрыт для расширения.

---

### 3.8. Файлы `components/responses/*.yaml` — где они проявляются

Эти шесть файлов сами по себе типов не порождают — все они ссылаются на один
и тот же `error.yaml`. Их роль в том, **какой код к какому типу привязать**:

| Файл | Используется в | Порождает тип |
|------|----------------|---------------|
| [bad_request.yaml](../shared/api/order/v1/components/responses/bad_request.yaml) | все 4 операции, код `400` | `*BadRequest` ×4 |
| [component_not_found.yaml](../shared/api/order/v1/components/responses/component_not_found.yaml) | createOrder, код `404` | `CreateOrderNotFound` |
| [component_out_of_stock.yaml](../shared/api/order/v1/components/responses/component_out_of_stock.yaml) | createOrder, код `409` | `CreateOrderConflict` |
| [order_not_found.yaml](../shared/api/order/v1/components/responses/order_not_found.yaml) | get/pay/cancel, код `404` | `GetOrderNotFound` и др. |
| [order_already_paid.yaml](../shared/api/order/v1/components/responses/order_already_paid.yaml) | pay/cancel, код `409` | `PayOrderConflict`, `CancelOrderConflict` |
| [internal_server_error.yaml](../shared/api/order/v1/components/responses/internal_server_error.yaml) | все 4, код `500` | `*InternalServerError` ×4 |

Поле `example` внутри них в Go-код не попадает — оно нужно только для документации
(Swagger UI показывает эти примеры под каждым кодом).

---

### 3.9. Enum-файлы → типы, константы и валидаторы

**payment_method.yaml** ([исходник](../shared/api/order/v1/components/enums/payment_method.yaml)):

```yaml
type: string
enum: [CARD, SBP, CREDIT_CARD, INVESTOR_MONEY]
```

Породил три вещи:

1. Тип и константы — [oas_schemas_gen.go:525](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L525):
```go
type PaymentMethod string

const (
	PaymentMethodCARD          PaymentMethod = "CARD"
	PaymentMethodSBP           PaymentMethod = "SBP"
	PaymentMethodCREDITCARD    PaymentMethod = "CREDIT_CARD"     ← подчёркивание убрано!
	PaymentMethodINVESTORMONEY PaymentMethod = "INVESTOR_MONEY"
)
```

2. Валидатор — [oas_validators_gen.go:87](../shared/pkg/openapi/order/v1/oas_validators_gen.go#L87).
Именно он даёт 400 при `{"payment_method":"BITCOIN"}`.

3. Метод `AllValues()` — список всех значений.

**order_status.yaml** — по той же схеме:
[oas_schemas_gen.go:426](../shared/pkg/openapi/order/v1/oas_schemas_gen.go#L426) (тип
и 3 константы), [oas_validators_gen.go:51](../shared/pkg/openapi/order/v1/oas_validators_gen.go#L51) (валидатор).

Про имена констант: ogen склеивает слова без разделителей — `CREDIT_CARD` становится
`PaymentMethodCREDITCARD`. У protobuf-версии того же перечисления имя другое:
`paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD` (так именует buf). Строка в JSON
у обоих одна — `"CREDIT_CARD"`, но это **разные Go-типы**, между которыми нужна таблица
перевода: [handler.go:37](pkg/handler/handler.go#L37).

---

### 3.10. `params/order_uuid.yaml` → параметры пути

Один файл на 8 строк ([исходник](../shared/api/order/v1/params/order_uuid.yaml)) —
но на него ссылаются три операции, поэтому породилось три одинаковых структуры:

| Структура | Строка | Функция разбора | Строка |
|-----------|--------|-----------------|--------|
| `CancelOrderParams` | [:19](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L19) | `decodeCancelOrderParams` | [:35](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L35) |
| `GetOrderParams` | [:85](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L85) | `decodeGetOrderParams` | [:101](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L101) |
| `PayOrderParams` | [:151](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L151) | `decodePayOrderParams` | [:167](../shared/pkg/openapi/order/v1/oas_parameters_gen.go#L167) |

Каждая — с единственным полем `OrderUUID uuid.UUID`. Функция разбора вытаскивает кусок
из URL и парсит в UUID; при неудаче — 400, наш код не вызывается. Поэтому в
[handler.go:272](pkg/handler/handler.go#L272) `params.OrderUUID` уже гарантированно
валидный UUID.

---

### 3.11. `order.openapi.yaml` → таблица маршрутов

Список путей ([order.openapi.yaml:24-32](../shared/api/order/v1/order.openapi.yaml)) стал
деревом разбора URL в [oas_router_gen.go](../shared/pkg/openapi/order/v1/oas_router_gen.go).
Вызовы обработчиков:

| Путь + метод | Строка вызова в роутере |
|--------------|-------------------------|
| `POST /api/v1/orders` | [:72](../shared/pkg/openapi/order/v1/oas_router_gen.go#L72) |
| `GET /api/v1/orders/{order_uuid}` | [:105](../shared/pkg/openapi/order/v1/oas_router_gen.go#L105) |
| `POST /api/v1/orders/{order_uuid}/cancel` | [:144](../shared/pkg/openapi/order/v1/oas_router_gen.go#L144) |
| `POST /api/v1/orders/{order_uuid}/pay` | [:171](../shared/pkg/openapi/order/v1/oas_router_gen.go#L171) |

Роутер сгенерирован как дерево посимвольного сравнения — быстрее, чем перебор
регулярных выражений. Если путь не совпал ни с чем, вернётся 404 без вызова нашего кода.

---

### 3.12. Обратный индекс: вижу Go-элемент — откуда он?

Таблица для навигации в обратную сторону.

| Go-элемент | Файл | Родился из |
|------------|------|-----------|
| `Handler` (интерфейс) | `oas_server_gen.go` | четырёх `operationId` в `paths/*.yaml` |
| `Server`, `NewServer` | `oas_server_gen.go` | самого факта наличия `paths` |
| `UnimplementedHandler` | `oas_unimplemented_gen.go` | тех же `operationId` |
| `CreateOrderRes` и др. интерфейсы | `oas_interfaces_gen.go` | блоков `responses` каждой операции |
| `CreateOrderRequest` | `oas_schemas_gen.go:52` | [create_order_request.yaml](../shared/api/order/v1/components/create_order_request.yaml) |
| `CreateOrderResponse` | `oas_schemas_gen.go:104` | [create_order_response.yaml](../shared/api/order/v1/components/create_order_response.yaml) |
| `PayOrderRequest` | `oas_schemas_gen.go:491` | [pay_order_request.yaml](../shared/api/order/v1/components/pay_order_request.yaml) |
| `PayOrderResponse` | `oas_schemas_gen.go:506` | [pay_order_response.yaml](../shared/api/order/v1/components/pay_order_response.yaml) |
| `CancelOrderResponse` (пустая) | `oas_schemas_gen.go:31` | [cancel_order_response.yaml](../shared/api/order/v1/components/cancel_order_response.yaml) — там `type: object` без свойств |
| `OrderDto` | `oas_schemas_gen.go:300` | [order_dto.yaml](../shared/api/order/v1/components/order_dto.yaml) |
| `Error` | `oas_schemas_gen.go:134` | [errors/error.yaml](../shared/api/order/v1/components/errors/error.yaml) |
| 15 типов `*NotFound`/`*Conflict`/… | `oas_schemas_gen.go` | кодов в `responses` × ссылки на `error.yaml` |
| `OrderStatus` + константы | `oas_schemas_gen.go:426` | [enums/order_status.yaml](../shared/api/order/v1/components/enums/order_status.yaml) |
| `PaymentMethod` + константы | `oas_schemas_gen.go:525` | [enums/payment_method.yaml](../shared/api/order/v1/components/enums/payment_method.yaml) |
| `OptNilUUID`, `NewOptNilUUID` | `oas_schemas_gen.go:245`, `:237` | наличия `nullable: true` у uuid-полей |
| `OptNilPaymentMethod` | `oas_schemas_gen.go:182` | `nullable: true` у `payment_method` в `order_dto.yaml` |
| `*Params` структуры | `oas_parameters_gen.go` | [params/order_uuid.yaml](../shared/api/order/v1/params/order_uuid.yaml) |
| `Validate()` методы | `oas_validators_gen.go` | `enum`, `required` и `format` в схемах |
| `Encode`/`Decode` методы | `oas_json_gen.go` | каждой схемы (1625 строк на всё) |
| `ServeHTTP` | `oas_router_gen.go:35` | списка `paths` |
| `encode*Response` | `oas_response_encoders_gen.go` | блоков `responses` |
| `decode*Request` | `oas_request_decoders_gen.go` | блоков `requestBody` |

---

### 3.13. Проверить самому

Убедиться, что связь именно такая, можно экспериментом:

```bash
cd project

# 1. Добавь в shared/api/order/v1/paths/order_by_uuid.yaml новый код ответа:
#      '409':
#        $ref: '../components/responses/order_already_paid.yaml'

# 2. Перегенерируй
task ogen:gen

# 3. Посмотри, что появилось
grep -n "GetOrderConflict" shared/pkg/openapi/order/v1/oas_schemas_gen.go
grep -n "GetOrderConflict" shared/pkg/openapi/order/v1/oas_response_encoders_gen.go
```

Появятся новый тип и новая ветка `case *GetOrderConflict: w.WriteHeader(409)`. Верни
контракт как было и перегенерируй — они исчезнут. Это самый наглядный способ убедиться,
что весь код в `shared/pkg/openapi/` — производная от YAML-файлов, а не что-то,
существующее само по себе.

## Часть 4. Вторая генерация — gRPC-клиенты

Order не только принимает HTTP, но и **сам вызывает** два gRPC-сервиса. Для этого нужен
второй набор сгенерированного кода — уже из `.proto`, другим инструментом.

```
shared/proto/inventory/v1/inventory.proto        ← контракт (писали руками)
shared/proto/payment/v1/payment.proto
              │
              │  task proto:gen  →  ./bin/buf generate
              ▼
shared/pkg/proto/inventory/v1/
    ├── inventory.pb.go            структуры: Part, ListPartsRequest, PartType…
    └── inventory_grpc.pb.go       InventoryServiceClient, InventoryServiceServer
shared/pkg/proto/payment/v1/
    ├── payment.pb.go
    └── payment_grpc.pb.go
```

Из **одного** `.proto` рождаются **обе** стороны:

```
inventory.proto
    │
    ├──► InventoryServiceServer  ← реализует inventory/pkg/service/service.go
    │                               (это другой сервис, другая программа)
    │
    └──► InventoryServiceClient  ← вызывает   order/pkg/handler/handler.go
                                    (это наш сервис)
```

Именно поэтому клиент и сервер не могут разойтись: они порождены одним файлом.
Изменил `.proto` → перегенерировал → у обеих сторон изменились типы → несоответствие
всплывёт при компиляции, а не в бою.

### Итого в Order используется код от двух генераторов

| | ogen | buf |
|--|------|-----|
| Читает | `order.openapi.yaml` | `*.proto` |
| Команда | `task ogen:gen` | `task proto:gen` |
| Роль Order | **сервер** (принимает) | **клиент** (вызывает) |
| Что даёт | `Handler`, `Server`, DTO, ошибки | `InventoryServiceClient`, `PaymentServiceClient`, сообщения |
| Где лежит | `shared/pkg/openapi/order/v1/` | `shared/pkg/proto/**/` |

---

## Часть 5. Как сгенерированное соединяется с нашим кодом

### Шаг 1: наш тип объявляет, что реализует интерфейс

[handler.go:72-78](pkg/handler/handler.go):
```go
type handler struct {
	orderv1.UnimplementedHandler                          // встроенная заглушка
	inventoryClient inventoryv1.InventoryServiceClient    // от buf
	paymentClient   paymentv1.PaymentServiceClient        // от buf
	store           *orderStore                           // наше хранилище
}
```

**Встраивание (embedding)** — первая строка без имени поля. Это Go-механизм: методы
встроенного типа становятся методами внешнего. `UnimplementedHandler` — сгенерированная
заглушка ([oas_unimplemented_gen.go](../shared/pkg/openapi/order/v1/oas_unimplemented_gen.go)),
в которой все четыре метода возвращают «не реализовано».

Благодаря ей `handler` считается реализующим `Handler` **сразу**, даже если ты написал
только один метод. Свои методы «перекрывают» заглушку. Без встраивания код не
скомпилировался бы, пока не написаны все четыре.

### Шаг 2: передаём наш тип в сгенерированный сервер

[handler.go:93-95](pkg/handler/handler.go):
```go
func SetupServer(h *handler) (*orderv1.Server, error) {
	return orderv1.NewServer(h)
}
```

`orderv1.NewServer` ожидает аргумент типа `Handler` (интерфейс). Мы передаём `*handler`.
Компилятор проверяет, что у него есть все нужные методы — если нет, ошибка сборки.

### Шаг 3: отдаём сервер в стандартный HTTP-сервер Go

[main.go:82-92](cmd/main.go):
```go
orderServer, err := orderHandler.SetupServer(h)
...
httpServer := &http.Server{
	Addr:    httpAddress,
	Handler: orderServer,       // ← сюда
	...
}
```

`http.Server.Handler` принимает интерфейс `http.Handler` из стандартной библиотеки —
это тип с единственным методом `ServeHTTP(w, r)`. У `orderv1.Server` такой метод есть
([oas_router_gen.go:35](../shared/pkg/openapi/order/v1/oas_router_gen.go)) — значит, подходит.

Получается цепочка вложенности:

```
http.Server (стандартная библиотека Go)
    └── orderv1.Server (сгенерировал ogen) — роутинг, разбор, валидация
            └── *handler (написали мы) — бизнес-логика
                    ├── inventoryClient (сгенерировал buf) → сеть → Inventory
                    └── paymentClient   (сгенерировал buf) → сеть → Payment
```

---

## Часть 6. Полный путь одного запроса

Разберём `POST /api/v1/orders` максимально подробно — с указанием файла на каждом шаге.

### Запрос

```bash
curl -X POST localhost:8080/api/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440001",
       "engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}'
```

### Шаг 1 — стандартный HTTP-сервер Go

Принимает TCP-соединение, читает HTTP-запрос, применяет таймауты из
[main.go:86-91](cmd/main.go), вызывает `Handler.ServeHTTP(w, r)`.

### Шаг 2 — роутер ogen

Файл: [oas_router_gen.go](../shared/pkg/openapi/order/v1/oas_router_gen.go)

Сравнивает путь `/api/v1/orders` и метод `POST` со всеми известными комбинациями
из контракта. Находит операцию `createOrder`. Если бы пути не было — вернул бы 404
сам, не вызывая наш код.

### Шаг 3 — разбор тела

Файл: [oas_request_decoders_gen.go](../shared/pkg/openapi/order/v1/oas_request_decoders_gen.go)

```go
ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
if err != nil {
	return ..., errors.Wrap(err, "parse media type")     // → 400
}
switch {
case ct == "application/json":
	if r.ContentLength == 0 {
		return ..., validate.ErrBodyRequired              // → 400
	}
	...
```

Дальше разбирает JSON, проверяет наличие `hull_uuid`/`engine_uuid`, парсит UUID-ы.
Любая проблема — **400 клиенту, наш код не вызывается**.

Именно это сообщение ты видел, когда отправил POST без заголовка:
```
{"error_message":"operation PayOrder: decode request: parse media type: mime: no media type"}
```

### Шаг 4 — вызов нашего метода

Файл: [oas_handlers_gen.go:306](../shared/pkg/openapi/order/v1/oas_handlers_gen.go)

```go
response, err = s.h.CreateOrder(ctx, request)
```

`s.h` — это тот самый `*handler`, который мы передали в `NewServer`. Здесь управление
переходит в наш код.

### Шаг 5 — наша логика

Файл: [handler.go:150-213](pkg/handler/handler.go)

```go
// 1. Собрать список UUID тех деталей, что реально пришли
uuids := []string{req.HullUUID.String(), req.EngineUUID.String()}

shieldUUID, hasShield := req.ShieldUUID.Get()
if hasShield {
    uuids = append(uuids, shieldUUID.String())
}
weaponUUID, hasWeapon := req.WeaponUUID.Get()
if hasWeapon {
    uuids = append(uuids, weaponUUID.String())
}
```

`.String()` нужен потому, что в gRPC-контракте (`inventory.proto`) поле объявлено как
`repeated string uuids` — protobuf не знает про Go-тип `uuid.UUID`, только про примитивы.

```go
// 2. Один вызов Inventory на все детали сразу
listResp, err := h.inventoryClient.ListParts(ctx, &inventoryv1.ListPartsRequest{
    Uuids: uuids,
})
```

`h.inventoryClient` — сгенерированный buf-ом клиент. Этот вызов выглядит как обычный
вызов функции, но внутри: сериализация в бинарный protobuf → отправка по HTTP/2 на
`localhost:50051` → ожидание → десериализация ответа.

### Шаг 6 — на стороне Inventory

Это уже **другая программа**, другой процесс. Её gRPC-фреймворк принимает запрос,
десериализует и вызывает `ListParts` из `inventory/pkg/service/service.go` — метод,
написанный на предыдущем этапе работы. Тот ищет детали в своей `map` и возвращает результат.

### Шаг 7 — обработка результата

```go
// 3. Ошибка от Inventory → соответствующий HTTP-код
if err != nil {
	st, _ := status.FromError(err)
	switch st.Code() {
	case codes.NotFound:
		return &orderv1.CreateOrderNotFound{Code: http.StatusNotFound, Message: "деталь не найдена"}, nil
	case codes.InvalidArgument:
		return &orderv1.CreateOrderBadRequest{Code: http.StatusBadRequest, ...}, nil
	default:
		return &orderv1.CreateOrderInternalServerError{Code: http.StatusInternalServerError, ...}, nil
	}
}
```

`status.FromError` распаковывает gRPC-ошибку. На стороне Inventory она была создана как
`status.Errorf(codes.NotFound, "деталь не найдена: %s", raw)` — код и текст доехали по сети.

**Важная деталь:** мы возвращаем `..., nil` — вторым значением `nil`, а не ошибку.
Для ogen ошибка во втором возвращаемом значении означает «непредвиденный сбой сервера»
и даёт 500. А ожидаемые ситуации (не найдено, конфликт) — это **нормальные ответы**,
их выражают типом первого значения.

```go
// 4. Проверить остаток и посчитать сумму
var totalPrice int64
for _, part := range listResp.GetParts() {
	if part.GetStockQuantity() <= 0 {
		return &orderv1.CreateOrderConflict{Code: http.StatusConflict, ...}, nil
	}
	totalPrice += part.GetPrice()
}
```

`.GetParts()`, `.GetPrice()` — сгенерированные buf-ом геттеры. Они безопасны при `nil`:
если объекта нет, вернут нулевое значение вместо паники.

```go
// 5. Собрать заказ
order := Order{
	OrderUUID:  uuid.New(),          // новый случайный UUID
	HullUUID:   req.HullUUID,        // берём ИЗ ЗАПРОСА
	EngineUUID: req.EngineUUID,
	TotalPrice: totalPrice,
	Status:     OrderStatusPendingPayment,
	CreatedAt:  time.Now(),
}
```

Почему поля берутся из запроса, а не из ответа Inventory: мы и так знаем, какой UUID
чем является — клиент сам это указал. `ListParts` нужен был только для проверки
существования, остатка и цены.

```go
// 6. Сохранить
h.store.mu.Lock()
h.store.orders[order.OrderUUID] = order
h.store.mu.Unlock()

return &orderv1.CreateOrderResponse{OrderUUID: order.OrderUUID, TotalPrice: totalPrice}, nil
```

### Шаг 8 — кодирование ответа

Файл: [oas_response_encoders_gen.go](../shared/pkg/openapi/order/v1/oas_response_encoders_gen.go)

Type switch видит `*CreateOrderResponse` → пишет статус **201** → сериализует в JSON
(код из `oas_json_gen.go`) → отдаёт клиенту.

```
HTTP/1.1 201 Created
Content-Type: application/json; charset=utf-8

{"order_uuid":"071ef6a8-8b66-4220-b5e6-045341c13dea","total_price":800000}
```

---

## Часть 7. Наш код — разбор по элементам

### 7.1. Хранилище и защита от гонок

[handler.go:59-70](pkg/handler/handler.go):
```go
type orderStore struct {
	mu     sync.RWMutex
	orders map[uuid.UUID]Order
}

func NewOrderStore() *orderStore {
	return &orderStore{orders: make(map[uuid.UUID]Order)}
}
```

HTTP-сервер в Go обрабатывает **каждый запрос в отдельной горутине** (лёгкий поток).
Значит, несколько запросов могут одновременно лезть в одну `map`. Одновременная запись
в map из разных горутин — это не просто «может быть неверный результат», а
`fatal error: concurrent map writes` с падением процесса.

`sync.RWMutex` — замок с двумя режимами:

| Метод | Кто может работать одновременно | Где используем |
|-------|--------------------------------|----------------|
| `RLock()` / `RUnlock()` | много читателей одновременно | `GetOrder` — только чтение |
| `Lock()` / `Unlock()` | только один, остальные ждут | `CreateOrder`, `PayOrder`, `CancelOrder` |

### 7.2. Почему в PayOrder один Lock на весь метод

[handler.go:215-224](pkg/handler/handler.go):
```go
func (h *handler) PayOrder(...) (orderv1.PayOrderRes, error) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	// ... чтение, проверка, вызов Payment, запись — всё под одним замком
```

Соблазн: взять `RLock` на чтение, отпустить, потом взять `Lock` на запись. Так делать нельзя:

```
Горутина A                          Горутина B
──────────                          ──────────
RLock, читает статус                                        статус = PENDING_PAYMENT
статус == PENDING_PAYMENT ✓
RUnlock
                                    RLock, читает статус     статус всё ещё PENDING_PAYMENT
                                    статус == PENDING_PAYMENT ✓   ← прошла проверку!
                                    RUnlock
Lock, пишет PAID, Unlock
                                    Lock, пишет PAID, Unlock
                                    ↑ заказ оплачен ДВАЖДЫ
```

Это называется **гонка check-then-write** (проверил-потом-записал). Лечится тем, что
проверка и запись происходят под одним непрерывным замком.

**`defer`** — Go-механизм: отложить выполнение до выхода из функции. `defer Unlock()`
гарантирует разблокировку при любом из шести `return` в методе. Без него пришлось бы
писать `Unlock()` перед каждым — и рано или поздно забыть.

### 7.3. Копия против ссылки

[handler.go:261-265](pkg/handler/handler.go):
```go
order, ok := h.store.orders[params.OrderUUID]     // получили КОПИЮ
...
order.Status = OrderStatusPaid                     // изменили копию
h.store.orders[params.OrderUUID] = order           // ← без этой строки изменение потеряется
```

В Go при чтении из `map[K]V` (где `V` — структура) возвращается **копия** значения, а не
ссылка на него. Изменение копии никак не влияет на содержимое map. Поэтому обязательна
явная запись обратно.

### 7.4. Два разных enum PaymentMethod

[handler.go:37-42](pkg/handler/handler.go):
```go
var paymentMethodToProto = map[orderv1.PaymentMethod]paymentv1.PaymentMethod{
	orderv1.PaymentMethodCARD:          paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	orderv1.PaymentMethodSBP:           paymentv1.PaymentMethod_PAYMENT_METHOD_SBP,
	orderv1.PaymentMethodCREDITCARD:    paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD,
	orderv1.PaymentMethodINVESTORMONEY: paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY,
}
```

| | `orderv1.PaymentMethod` | `paymentv1.PaymentMethod` | `handler.PaymentMethod` |
|--|--------------------------|---------------------------|--------------------------|
| Откуда | `payment_method.yaml` → ogen | `payment.proto` → buf | наш, [handler.go:28](pkg/handler/handler.go) |
| Базовый тип | `string` | `int32` | `string` |
| Где применяется | HTTP JSON | gRPC-вызов | внутри `Order` |

Три типа для одного понятия — потому что три независимых контракта (HTTP-контракт,
gRPC-контракт, внутренняя модель). Между `string`-типами конвертация прямая:

```go
pm := PaymentMethod(req.PaymentMethod)     // handler.go:261 — приведение типа
```

А вот в protobuf-enum (`int32`) прямого приведения нет — значения не совпадают
(`"CARD"` против `1`). Отсюда таблица.

**Почему map, а не `switch`:** это чистое соответствие один-к-одному, то есть данные,
а не логика. Map читается как таблица, добавление пятого способа оплаты — одна строка.
Плюс `ok` из map-lookup даёт защиту от неизвестного значения бесплатно.

---

## Часть 8. Инфраструктура main.go

[main.go:43-48](cmd/main.go):
```go
func main() {
	if err := run(); err != nil {
		slog.Error("сервис завершился с ошибкой", "error", err)
		os.Exit(1)
	}
}
```

**Почему вся работа в `run()`, а не прямо в `main()`:** `os.Exit()` завершает процесс
мгновенно, **не выполняя отложенные вызовы `defer`**. Если бы `os.Exit(1)` стоял внутри,
после `defer inventoryConn.Close()` — соединение никогда бы не закрылось. Вынеся `os.Exit`
в `main`, мы гарантируем: сначала `run()` возвращает управление (выполняя все `defer`),
и только потом процесс завершается. Это стандартный Go-паттерн, и линтер `gocritic`
за ним следит.

[main.go:57-70](cmd/main.go) — gRPC-клиенты с keepalive:
```go
keepaliveParams := grpc.WithKeepaliveParams(keepalive.ClientParameters{
	Time:                grpcKeepaliveTime,     // 10s — пинг раз в 10 секунд
	Timeout:             grpcKeepaliveTimeout,  // 3s  — нет ответа → соединение мёртвое
	PermitWithoutStream: true,                  // пинговать даже без активных вызовов
})
```

Без keepalive клиент не заметит обрыва соединения, пока не сделает реальный вызов — и тот
зависнет надолго. Пинги обнаруживают проблему заранее.

[main.go:86-91](cmd/main.go) — таймауты HTTP:
```go
httpServer := &http.Server{
	Addr:              httpAddress,
	Handler:           orderServer,
	ReadHeaderTimeout: 5 * time.Second,    // ← защита от Slowloris
	ReadTimeout:       15 * time.Second,
	WriteTimeout:      15 * time.Second,
	IdleTimeout:       60 * time.Second,
}
```

Удобная функция `http.ListenAndServe(addr, handler)` создаёт сервер **без единого
таймаута** (все нули = «ждать бесконечно»). `ReadHeaderTimeout` критичен: без него
атакующий открывает тысячи соединений и шлёт заголовки по одному раз в 10 секунд,
никогда не завершая запрос. Каждое такое соединение держит файловый дескриптор и
горутину — лимит ОС исчерпывается, обычные клиенты получают отказ. Это атака Slowloris.

[main.go:96-114](cmd/main.go) — graceful shutdown:
```go
serverErrCh := make(chan error, 1)

go func() {
	slog.Info("запуск OrderService", "адрес", httpAddress)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverErrCh <- err
	}
}()

ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

select {
case err := <-serverErrCh:
	return err
case <-ctx.Done():
}
```

Построчно:
- **`go func() {...}()`** — запуск в горутине. `ListenAndServe()` блокирует выполнение,
  без горутины следующие строки никогда не выполнились бы.
- **`make(chan error, 1)`** — канал, способ передать значение между горутинами. Число `1` —
  размер буфера: горутина может отправить ошибку и не ждать, пока её прочитают.
- **`errors.Is(err, http.ErrServerClosed)`** — эту ошибку `ListenAndServe` возвращает при
  штатной остановке через `Shutdown`. Не сбой, отфильтровываем.
- **`signal.NotifyContext`** — создаёт контекст, который отменится при получении сигнала.
  `SIGINT` — это `Ctrl+C`, `SIGTERM` — что шлёт Docker/Kubernetes при остановке контейнера.
- **`select`** — ждёт **несколько** каналов одновременно, срабатывает на том, где первым
  появится значение. Здесь: либо сервер упал сам, либо пришёл сигнал остановки.

[main.go:117-122](cmd/main.go):
```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
defer cancel()

if err := httpServer.Shutdown(shutdownCtx); err != nil {
	slog.Error("ошибка при остановке сервера", "error", err)
}
```

Обрати внимание: базой для `shutdownCtx` служит `context.Background()`, а **не** тот `ctx`
из `NotifyContext` — тот уже отменён (сигнал же пришёл), и `Shutdown` с ним завершился бы
мгновенно, оборвав активные запросы.

`Shutdown` по шагам: перестаёт принимать новые соединения → закрывает простаивающие →
ждёт завершения активных запросов → если за 30 секунд не уложились, обрывает.

**Порядок закрытия ресурсов.** `defer inventoryConn.Close()` и `defer paymentConn.Close()`
зарегистрированы в начале `run()`. Go выполняет отложенные вызовы в обратном порядке при
выходе из функции — то есть **после** того, как `Shutdown` вернул управление. Это верная
последовательность: сначала дожидаемся HTTP-запросов (которые прямо сейчас могут вызывать
Inventory/Payment), и только потом рвём gRPC-соединения.

---

## Часть 9. Шпаргалка «где что искать»

| Хочу понять | Смотреть |
|-------------|----------|
| Какие есть эндпоинты | [order.openapi.yaml](../shared/api/order/v1/order.openapi.yaml) → `paths` |
| Какие поля в запросе/ответе | `shared/api/order/v1/components/*.yaml` |
| Как выглядит Go-структура | `shared/pkg/openapi/order/v1/oas_schemas_gen.go` |
| Какие методы надо реализовать | `oas_server_gen.go` → `type Handler interface` |
| Какой тип даёт какой HTTP-код | `oas_response_encoders_gen.go` → `encode*Response` |
| Что проверяется до моего кода | `oas_request_decoders_gen.go`, `oas_validators_gen.go` |
| Бизнес-логика | [handler.go](pkg/handler/handler.go) |
| Запуск, соединения, таймауты | [main.go](cmd/main.go) |
| Как правильно себя ведёт сервис | [tests/api_test.go](tests/api_test.go) |

---

## Часть 10. Что делать при изменении API

Порядок всегда один: **контракт → генерация → код**.

```bash
# 1. Правим контракт
#    HTTP:  shared/api/order/v1/**.yaml
#    gRPC:  shared/proto/**/*.proto

# 2. Перегенерируем
task ogen:gen      # для HTTP
task proto:gen     # для gRPC

# 3. Собираем — компилятор покажет, где наш код разошёлся с новым контрактом
go build ./order/...

# 4. Правим handler.go под новые сигнатуры/типы

# 5. Проверяем
task test:api
task format && task lint
```

Никогда не редактируй файлы в `shared/pkg/` — там стоит `// Code generated … DO NOT EDIT`,
и при следующей генерации всё затрётся (в конфиге стоит флаг `-clean`, который сначала
удаляет папку).

---

## Часть 11. Запуск и проверка

Нужны все три сервиса — Order без них вернёт 500 на первом же запросе:

```bash
go run ./inventory/cmd    # терминал 1 → :50051
go run ./payment/cmd      # терминал 2 → :50052
go run ./order/cmd        # терминал 3 → :8080
```

Полный цикл (UUID подставляется автоматически):

```bash
ORDER=$(curl -s -X POST localhost:8080/api/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440001",
       "engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}' \
  | sed 's/.*"order_uuid":"\([^"]*\)".*/\1/')

curl -s localhost:8080/api/v1/orders/$ORDER            # 200, PENDING_PAYMENT

curl -s -X POST localhost:8080/api/v1/orders/$ORDER/pay \
  -H 'Content-Type: application/json' -d '{"payment_method":"CARD"}'

curl -s localhost:8080/api/v1/orders/$ORDER            # 200, PAID
curl -i -X POST localhost:8080/api/v1/orders/$ORDER/cancel   # 409
```

Проверка ошибок:

```bash
# несуществующая деталь → 404
curl -i -X POST localhost:8080/api/v1/orders -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440099","engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}'

# деталь без остатка (Плазменный корпус, stock=0) → 409
curl -i -X POST localhost:8080/api/v1/orders -H 'Content-Type: application/json' \
  -d '{"hull_uuid":"550e8400-e29b-41d4-a716-446655440007","engine_uuid":"550e8400-e29b-41d4-a716-446655440003"}'

curl -i localhost:8080/api/v1/orders/not-a-uuid        # 400 — поймал ogen
```

⚠️ Данные живут в памяти процесса. После перезапуска Order все заказы исчезают —
`order_uuid` из прошлого запуска даст 404.

Автотесты поднимают все три сервиса **внутри одного процесса** (gRPC через `bufconn` —
соединение в памяти без реальных портов, HTTP через `httptest`), поэтому запускать
`go run` для них не нужно:

```bash
task test:api
```

Graceful shutdown: `Ctrl+C` в терминале Order → в логах «получен сигнал остановки…»,
затем «сервер остановлен», процесс завершается сам.
