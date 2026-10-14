# Week 2 — как это работает

Разбор примеров второй недели: тестирование и слоистая архитектура. Восемь примеров, которые
читаются как одна нарастающая линия — от голого `if` внутри функции до параллельных тестов
слоёв и E2E-прогона всего сервиса через in-memory gRPC.

У каждой поддиректории есть свой короткий README — там сказано «что смотреть». Этот документ
про другое: **почему** так, чем один подход отличается от соседнего и в какой момент нужно
переключаться с одного на другой.

---

## Карта примеров

| # | Директория | Новая идея по сравнению с предыдущим |
|---|-----------|--------------------------------------|
| 1 | [`unit_tests/1_easy_test/`](unit_tests/1_easy_test/) | Пакет `testing` вместо самодельного «теста» |
| 2 | [`unit_tests/2_common_unit_test/`](unit_tests/2_common_unit_test/) | Table-driven: таблица кейсов + `t.Run` |
| 3 | [`unit_tests/3_testify_unit_test/`](unit_tests/3_testify_unit_test/) | testify: `require` vs `assert`, проверка ошибок |
| 4 | [`unit_tests/4_unit_test_with_stubs/`](unit_tests/4_unit_test_with_stubs/) | Зависимость через интерфейс, подмена **стабом** |
| 5 | [`unit_tests/5_unit_test_with_mocks/`](unit_tests/5_unit_test_with_mocks/) | **Мок** вместо стаба: mockery v3, верификация вызовов |
| — | [`layers/`](layers/) | Чистая архитектура — сервис, который дальше тестируем по слоям |
| 6 | [`unit_tests/6_unit_test_in_clean_arch/`](unit_tests/6_unit_test_in_clean_arch/) | `testify/suite`: setup/teardown, тесты слоёв |
| 7 | [`unit_tests/7_parallel_tests/`](unit_tests/7_parallel_tests/) | То же самое, но параллельно и table-driven |

Первые пять примеров — про тестирование **функции**. Примеры 6, 7 и `layers/` — про
тестирование **слоёв** одного и того же сервиса регистрации наблюдений НЛО.

### Пирамида этой недели

```
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│   E2E (bufconn)          layers/tests/api_test.go                        │
│   ─────────────          весь стек живой: api → service → repository     │
│   мокаем: ничего         только внешний клиент погоды — stub             │
│                          ▲                                               │
│   Unit сервисного слоя   │  мокаем UFORepository                         │
│   ────────────────────   │  проверяем: бизнес-логику (credibility),      │
│                          │  обработку ошибок репозитория                 │
│                          ▲                                               │
│   Unit API-слоя          │  мокаем UFOService                            │
│   ─────────────          │  проверяем: валидацию, конвертацию            │
│                          │  proto ↔ model, маппинг ошибок в gRPC-коды    │
│                          ▲                                               │
│   Unit чистых функций    │  мокать нечего                                │
│   ───────────────────    │  CalculateCreditScore, calculateCredibility   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

Главное правило, которое отсюда следует: **мокается ровно один слой — тот, что лежит прямо
под тестируемым**. API-тест не знает про репозиторий, сервисный тест не знает про proto.
А репозиторий вообще не покрывают юнит-тестами — он проверяется E2E, в реальной цепочке.

---

# Часть I. Лестница 1→5: от `if` до мока

## 1. `1_easy_test` — зачем вообще пакет `testing`

Функция — одна строка:

```go
// isStupid возвращает true, если у человека нет мозга
func isStupid(hasBrain bool) bool {
	return !hasBrain
}
```

В [`main.go`](unit_tests/1_easy_test/main.go) рядом лежит намеренный антипаттерн — «тест»,
написанный руками:

```go
func testIsStupid() {
	slog.Info("тест isStupid(false) — нет мозга, значит тупой")
	if !isStupid(false) {
		slog.Error("тест провален: ожидали true")
		return
	}
	slog.Info("тест пройден")
	...
}
```

Что с ним не так — по пунктам:

| Проблема | Следствие |
|----------|-----------|
| Функция нигде не вызывается | Она никогда не выполнится; «покрытие» существует только на бумаге |
| Нет интеграции с `go test` | CI не узнает о провале — процесс завершится с кодом 0 |
| Провал = запись в лог | Нужно глазами читать вывод, чтобы понять, всё ли хорошо |
| Живёт в `main.go` | Тестовый код уезжает в продовый бинарник |

Правильная версия — [`main_test.go`](unit_tests/1_easy_test/main_test.go):

```go
func TestIsStupid(t *testing.T) {
	if !isStupid(false) {
		t.Error("ожидали true для hasBrain=false")
	}
	if isStupid(true) {
		t.Error("ожидали false для hasBrain=true")
	}
}
```

Разница не в коде проверок — она почти та же. Разница в трёх соглашениях, которые понимает
тулчейн Go:

1. файл заканчивается на `_test.go` → не попадает в обычную сборку;
2. функция называется `TestXxx` и принимает `*testing.T` → её найдёт и вызовет `go test`;
3. `t.Error()` помечает тест проваленным → ненулевой код возврата → красный CI.

Никаких библиотек здесь ещё нет — только голый `testing`.

## 2. `2_common_unit_test` — table-driven

Тестируемая функция стала настоящей: [`CalculateCreditScore`](unit_tests/2_common_unit_test/credit_score/calculate_credit_score.go)
считает кредитный рейтинг по шести полям клиента через цепочку `switch`.

Это **чистая функция** — нет глобального состояния, нет обращений в сеть и на диск, результат
зависит только от аргумента. Идеальный кандидат для юнит-теста: подставляй входы и сравнивай
выходы, готовить нечего.

Много веток → много кейсов. Писать под каждый отдельную `TestXxx` — копипаста. Стандартное
решение в Go — таблица:

```go
tests := []struct {
	name     string
	client   Client
	expected int
}{
	{
		name:     "мужчина инженер 30 лет",
		client:   Client{"male", 30, "engineer", 7, 60000, 0},
		expected: 750,
	},
	...
}

for _, tc := range tests {
	t.Run(tc.name, func(t *testing.T) {
		result := CalculateCreditScore(tc.client)
		if result != tc.expected {
			t.Errorf("CalculateCreditScore(%#v) = %d; want %d", tc.client, result, tc.expected)
		}
	})
}
```

Что это даёт:

- **новый кейс = новая строка в таблице**, а не новая функция;
- `t.Run(tc.name, ...)` создаёт именованный подтест — в `go test -v` видно, какой именно кейс
  упал, и его можно запустить отдельно: `go test -run 'TestCalculateCreditScore/мужчина_инженер_30_лет'`;
- один подтест не мешает другим: `t.Error` внутри `t.Run` валит только этот подтест.

Ассерты пока ручные — `if` + `t.Errorf`. Это последний пример без testify, и он нужен, чтобы
было с чем сравнивать.

## 3. `3_testify_unit_test` — require vs assert

Та же функция, но теперь она умеет отказывать:

```go
var (
	ErrInvalidAge    = errors.New("некорректный возраст")
	ErrInvalidSalary = errors.New("некорректная зарплата")
)

func CalculateCreditScore(client Client) (int, error) {
	if client.Age < 0 {
		return 0, fmt.Errorf("%w: %d", ErrInvalidAge, client.Age)
	}
	...
}
```

Обрати внимание на `%w` — это **обёртывание** ошибки. Наружу уезжает текст с деталями
(`некорректный возраст: -1`), но внутри сохраняется исходная `ErrInvalidAge`, до которой можно
докопаться через `errors.Is`. Если написать `%v` вместо `%w`, связь потеряется и проверка в
тесте перестанет работать.

Тест переезжает на testify. Два пакета, и путать их нельзя:

| Пакет | Что вызывает при провале | Поведение |
|-------|--------------------------|-----------|
| `require` | `t.FailNow()` → `runtime.Goexit()` | Текущий тест/подтест **останавливается немедленно**. Остальные продолжают |
| `assert` | `t.Fail()` | Тест **продолжается**, собирая все расхождения |

**Правило: `require` — для «ворот» (gates), `assert` — для «проверок» (checks).**

```go
result, err := CalculateCreditScore(client)
require.NoError(t, err)           // gate: если err != nil, дальше проверять нечего
assert.Equal(t, expected, result) // check: сравниваем значение
```

Логика простая: если предусловие не выполнено (ошибка вместо результата, `nil` вместо
структуры), то следующие строки либо бессмысленны, либо уронят тест паникой по nil-указателю.
Такие проверки — `require`. А когда сверяешь пять полей ответа, хочется увидеть **все** пять
расхождений сразу, а не чинить их по одному — это `assert`.

Проверка ошибок:

```go
_, err := CalculateCreditScore(client)
require.Error(t, err)
require.ErrorIs(t, err, ErrInvalidAge)
```

`ErrorIs` разворачивает цепочку `%w` и ищет в ней конкретную ошибку. Это устойчивее, чем
сравнивать текст сообщения: текст можно переписать, не сломав ни одного теста.

## 4. `4_unit_test_with_stubs` — стаб

Появляется зависимость. Функция больше не чистая — она ходит во внешний сервис погоды:

```go
type WeatherCenterClient interface {
	SetTemperature(city string, temperature float32)
	GetTemperature(city string) (float32, error)
}

func getWeatherAdvice(client WeatherCenterClient, city string) (string, error) {
	temperature, err := client.GetTemperature(city)
	if err != nil {
		return "", fmt.Errorf("не удалось получить температуру у города %s: %w", city, err)
	}
	switch {
	case temperature >= -71.0 && temperature <= -40.0:
		return "Лучше не суйся на улицу", nil
	...
	}
}
```

Ключевое здесь — **первый аргумент это интерфейс, а не конкретный тип**. Именно это делает
функцию тестируемой: в проде передаём настоящий клиент, в тесте — что угодно, лишь бы оно
реализовывало два метода.

```
        Тестируемый код                       Зависимость
   getWeatherAdvice(client, city) ──► WeatherCenterClient (интерфейс)
                                              │
                          ┌───────────────────┴───────────────────┐
                          │                                       │
                  weather_center.WeatherCenter        weather_center_stub.WeatherCenter
                  (боевая, пустая map,                (захардкоженные 8 городов,
                   заполняется через Set)              Set игнорируется)
```

Стаб — [`weather_center_stub/weather_center.go`](unit_tests/4_unit_test_with_stubs/weather_center_stub/weather_center.go):

```go
func NewWeatherCenter() *WeatherCenter {
	return &WeatherCenter{
		temperatureByCity: map[string]float32{
			"Москва":   25.0,
			"Якутск":   -50.0,
			"Сочи":     30.0,
			...
		},
	}
}

func (w *WeatherCenter) SetTemperature(_ string, _ float32) {
	// Стаб игнорирует установку температуры — значения захардкожены
}
```

**Стаб = заранее заданные ответы.** Он отвечает на вопрос «что вернёт зависимость», и на этом
его обязанности заканчиваются.

Тест выглядит как пример 2 плюс один стаб на все кейсы:

```go
clientStub := weatherCenterStub.NewWeatherCenter()

for _, tc := range tests {
	t.Run(tc.name, func(t *testing.T) {
		res, err := getWeatherAdvice(clientStub, tc.city)
		require.NoError(t, err)
		require.Equal(t, tc.expected, res)
	})
}
```

Чего стаб **не** умеет — и это причина существования примера 5:

- проверить, что `GetTemperature` вообще вызвали;
- проверить, с каким аргументом её вызвали;
- проверить, что её вызвали ровно один раз, а не трижды;
- вернуть разные значения в разных кейсах (поведение одно на весь файл — оно в конструкторе).

Последний пункт особенно неприятен: чтобы добавить кейс «-71 градус», придётся дописывать
город в стаб, то есть править тестовый двойник ради одного теста.

## 5. `5_unit_test_with_mocks` — мок

Функция та же. Меняется только двойник: вместо рукописного стаба — мок, сгенерированный
**mockery v3**.

### Конфигурация — [`.mockery.yaml`](unit_tests/5_unit_test_with_mocks/.mockery.yaml)

```yaml
template: testify                      # шаблон с expecter API (.EXPECT())
formatter: goimports
dir: "{{.InterfaceDir}}/mocks"         # мок кладём рядом с интерфейсом, в подпапку mocks
filename: "mock_{{ .InterfaceName | snakecase }}.go"
pkgname: "mocks"
structname: "{{.InterfaceName}}"       # mocks.WeatherCenterClient
force-file-write: true

packages:
  github.com/.../5_unit_test_with_mocks:
    config:
      include-interface-regex: ".*Client"   # мокаем всё, что кончается на Client
```

Регексп вместо перечисления имён — добавил новый интерфейс `SomethingClient`, запустил
`task mocks:gen`, мок появился сам. Генерация — задача Taskfile:

```bash
task mocks:gen       # ставит mockery нужной версии в bin/ и запускает
task test            # go test -v ./...
```

Сгенерированные моки **коммитятся** — иначе `go test ./...` на чистом клоне не соберётся.

### Тест

```go
tests := []struct {
	name                    string
	city                    string
	expected                string
	errCheck                func(t *testing.T, err error)
	weatherCenterClientMock func(t *testing.T) WeatherCenterClient
}{
	{
		name:     "Температура +25 градусов",
		city:     city,
		expected: "Отличная погода для прогулок",
		errCheck: func(t *testing.T, err error) {
			require.NoError(t, err)
		},
		weatherCenterClientMock: func(t *testing.T) WeatherCenterClient {
			mockClient := mocks.NewWeatherCenterClient(t)
			mockClient.EXPECT().GetTemperature(city).Return(float32(25), nil)
			return mockClient
		},
	},
	...
}
```

Три вещи, которых не было в примере 4:

**1. Мок — часть кейса, а не общий на файл.** В таблице лежит не готовый двойник, а
*фабрика* `func(t *testing.T) WeatherCenterClient`. Каждый кейс собирает свой мок со своим
поведением. Кейс «-15 градусов» не требует править чужой код — он полностью самодостаточен.

**2. Expecter API описывает вызов, а не только ответ.**

```go
mockClient.EXPECT().GetTemperature(city).Return(float32(25), nil)
//                  ^^^^^^^^^^^^^^^^^^^^^ метод и ожидаемый аргумент
//                                         ^^^^^^^^^^^^^^^^^^^^^^ что вернуть
```

Если код вызовет `GetTemperature` с другим городом — тест упадёт прямо в месте вызова.
Стаб такое пропустил бы.

**3. Невыполненное ожидание = провал.** `mocks.NewWeatherCenterClient(t)` принимает `*testing.T`
и регистрирует `t.Cleanup()`, который после теста проверяет: все ли настроенные ожидания
действительно сработали. Это ловит самый коварный класс багов — когда тест зелёный, потому
что тестируемый код вообще не дошёл до зависимости.

**Про `gofakeit`.** Город берётся случайным: `city := gofakeit.City()`. Это не про
«потестировать на разных данных» — значение одно на весь прогон. Это **сигнал читателю**:
конкретное значение здесь не важно, важен факт, что в мок и в функцию уехало одно и то же.
Если в тесте видишь захардкоженное `"Москва"` — читатель обязан задуматься, есть ли в коде
что-то, завязанное на Москву. `gofakeit.City()` снимает этот вопрос.

Кейс с ошибкой показывает, зачем нужно `errCheck`:

```go
{
	name:     "Город не найден",
	expected: "",
	errCheck: func(t *testing.T, err error) {
		require.Error(t, err)
		require.ErrorIs(t, err, weather_center.ErrCityNotFound)
	},
	weatherCenterClientMock: func(t *testing.T) WeatherCenterClient {
		mockClient := mocks.NewWeatherCenterClient(t)
		mockClient.EXPECT().GetTemperature(city).Return(float32(0), weather_center.ErrCityNotFound)
		return mockClient
	},
},
```

Проверяется, что ошибка зависимости **пробрасывается наружу через `%w`** и остаётся узнаваемой
для `errors.Is`. Один и тот же приём, что в примере 3, только ошибка теперь приходит извне.

## 6. Стаб или мок — как выбрать

| | Стаб (пример 4) | Мок (пример 5) |
|---|---|---|
| Поведение | Захардкожено в конструкторе | Настраивается в каждом кейсе |
| Проверка вызовов | Нет | Да: метод, аргументы, количество |
| Откуда берётся | Пишешь руками | Генерирует mockery |
| Цена добавления кейса | Правка общего двойника | Одна строка `EXPECT()` |
| Что ломается при рефакторинге интерфейса | Компиляция стаба | Перегенерация — `task mocks:gen` |

Практическое правило:

- нужен только **результат** зависимости → стаб. Он проще, читается без знания API mockery,
  хорош для «фоновых» зависимостей вроде клиента погоды в `layers/`;
- нужно проверить **факт и параметры взаимодействия** → мок. Особенно когда взаимодействие и
  есть суть теста: «сервис должен сходить в репозиторий ровно с этой моделью».

---

# Часть II. Слои и тесты слоёв

## 7. `layers/` — архитектура, на которой всё стоит

Сервис регистрации наблюдений НЛО. Тот же домен, что в 6 и 7, но здесь он показан целиком,
без тестового акцента.

```
                   gRPC-запрос (ufo.v1.UFOService)
                              │
                              ▼
   ┌──────────────────────────────────────────────────────────┐
   │ API          internal/api/ufo/v1/                        │
   │              валидация, proto ↔ input/model,             │
   │              маппинг доменных ошибок в gRPC-коды         │
   └──────────────────────────┬───────────────────────────────┘
                              │  зависит от интерфейса UFOService
                              ▼
   ┌──────────────────────────────────────────────────────────┐
   │ Service      internal/service/ufo/                       │
   │              бизнес-логика: credibility, оркестрация     │
   └──────────┬──────────────────────────────┬────────────────┘
              │ UFORepository                │ WeatherClient
              ▼                              ▼
   ┌────────────────────────┐   ┌──────────────────────────────┐
   │ Repository             │   │ Внешний клиент погоды        │
   │ in-memory + RWMutex    │   │ stub/ или grpc/ — выбор в    │
   │ свои record-модели     │   │ main.go                      │
   └────────────────────────┘   └──────────────────────────────┘
```

### 7.1. Зависимости объявляет тот, кто их использует

Каждый слой описывает соседа снизу в собственном `deps.go`:

```go
// internal/api/ufo/v1/deps.go
type UFOService interface {
	Create(ctx context.Context, in input.CreateSightingInput) (string, error)
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, in input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}
```

```go
// internal/service/ufo/deps.go
type UFORepository interface {
	Create(ctx context.Context, sighting model.Sighting) error
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, sighting model.Sighting) error
	Delete(ctx context.Context, uuid string) error
}

type WeatherClient interface {
	GetCurrentWeather(ctx context.Context, location string) (model.WeatherConditions, error)
}
```

Это не бюрократия — это **точки подмены**. Ровно эти интерфейсы попадут в `.mockery.yaml`
примеров 6 и 7 и станут моками. Нет интерфейса — нет юнит-теста слоя.

Конструкторы принимают интерфейсы и возвращают неэкспортируемые структуры:

```go
type api struct {
	ufov1.UnimplementedUFOServiceServer
	ufoService UFOService
}

func NewAPI(ufoService UFOService) *api { return &api{ufoService: ufoService} }
```

Сборка — в [`cmd/grpc_server/main.go`](layers/cmd/grpc_server/main.go), руками, без DI-фреймворка:

```go
repo := ufoRepository.NewRepository()
weatherClient := weatherStub.New()
service := ufoService.NewService(repo, weatherClient)
api := ufov1API.NewAPI(service)
```

Запомни эту цепочку — она дословно повторится в E2E-тесте (раздел 10).

### 7.2. Конвертеры на каждой границе

Модель не путешествует между слоями в чужом формате. На каждой границе — свой пакет:

| Граница | Пакет | Что делает |
|---------|-------|------------|
| proto ↔ домен | `internal/api/converter/` | `CreateRequestToInput`, `SightingToProto` |
| домен ↔ хранилище | `internal/repository/converter/` | `SightingToRepoModel`, `SightingToModel` |

Транспортный слой оперирует wrapper-типами (`wrapperspb.StringValue` и т.п. — они нужны,
чтобы отличить «поле не передали» от «передали пустую строку»), а домен — обычными
Go-указателями. Конвертер и есть этот перевод:

```go
func CreateRequestToInput(req *ufov1.CreateRequest) input.CreateSightingInput {
	var color *string
	if req.Color != nil {
		color = new(req.Color.Value)
	}
	...
}
```

`new(req.Color.Value)` — это форма builtin `new` из Go 1.26: принимает **значение** и
возвращает указатель на его копию. Раньше для этого писали хелпер или тянули `samber/lo.ToPtr`
(README `layers/` до сих пор упоминает `lo`, но в коде его уже нет — модуль на `go 1.26.0`).

Почему это важно для тестов: в примерах 6 и 7 API-тест ожидает от мока
не собранный руками `input`, а результат того же конвертера:

```go
expectedModelInfo := converter.CreateRequestToInput(req)
s.ufoService.EXPECT().Create(s.ctx, expectedModelInfo).Return(expectedUUID, nil)
```

Тест проверяет «API вызвал сервис с тем, что даёт конвертер», а не переписывает логику
конвертера у себя — иначе он ломался бы при каждом добавлении поля.

### 7.3. Что живёт в каждом слое (и как это ловится тестом)

**API — валидация и коды ответа.**

```go
func (a *api) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	if req.GetLocation() == "" {
		return nil, status.Error(codes.InvalidArgument, "location не может быть пустым")
	}

	uuid, err := a.ufoService.Create(ctx, converter.CreateRequestToInput(req))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ошибка создания наблюдения: %v", err)
	}

	return &ufov1.CreateResponse{Uuid: uuid}, nil
}
```

Два тестовых сценария вытекают из кода буквально: пустой `Location` → `InvalidArgument` и
**сервис не вызван вовсе**; ошибка сервиса → `Internal`.

**Service — бизнес-логика.** Достоверность наблюдения считается по заполненности шести полей:

```go
func calculateCredibility(s model.Sighting) string {
	score := 0
	if s.ObservedAt != nil       { score++ }
	if s.Location != ""          { score++ }
	if s.Description != ""       { score++ }
	if s.Color != nil            { score++ }
	if s.Sound != nil            { score++ }
	if s.DurationSeconds != nil  { score++ }

	switch {
	case score >= 5: return "high"
	case score >= 3: return "medium"
	default:         return "low"
	}
}
```

Отсюда ровно три граничных кейса в тестах: 6 полей → `high`, 3 → `medium`, 1 → `low`.

**Service — graceful degradation.** В `layers/internal/service/ufo/create.go` погода
необязательна:

```go
weather, err := s.weatherClient.GetCurrentWeather(ctx, in.Location)
var weatherPtr *model.WeatherConditions
if err != nil {
	if errors.Is(err, errs.ErrWeatherUnavailable) {
		slog.WarnContext(ctx, "сервис погоды недоступен, продолжаем без данных о погоде",
			"location", in.Location)
	} else {
		return "", fmt.Errorf("получить погоду: %w", err)
	}
} else {
	weatherPtr = &weather
}
```

Одна известная ошибка (`ErrWeatherUnavailable`) деградирует до предупреждения, любая другая —
валит запрос. Тестировать это без мока невозможно: нужно уметь заставить зависимость вернуть
конкретную ошибку.

**Repository — потокобезопасность и мягкое удаление.**

```go
func (r *repository) Get(_ context.Context, uuid string) (model.Sighting, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSighting, ok := r.data[uuid]
	if !ok {
		return model.Sighting{}, errs.ErrSightingNotFound
	}
	if repoSighting.DeletedAt != nil {
		return model.Sighting{}, errs.ErrSightingNotFound
	}

	return repoConverter.SightingToModel(repoSighting), nil
}
```

Удалённая запись физически остаётся в map, но снаружи её не видно. Отдельно этот слой
юнит-тестами не покрывают — он проверяется E2E (раздел 10): мокать `map` бессмысленно, тест
превратился бы в переписывание реализации.

Доменные ошибки живут в одном месте — [`internal/errors/errors.go`](layers/internal/errors/errors.go):

```go
var (
	ErrSightingNotFound   = errors.New("наблюдение не найдено")
	ErrWeatherUnavailable = errors.New("сервис погоды недоступен")
)
```

## 8. `6_unit_test_in_clean_arch` — testify/suite

Та же архитектура (без клиента погоды — он не нужен для демонстрации), тесты API- и
сервисного слоя на `testify/suite`.

### 8.1. Жизненный цикл

```go
func TestAPISuite(t *testing.T) {
	suite.Run(t, new(APISuite))
}
```

```
suite.Run(t, new(APISuite))
   │
   ├─ SetupTest() ─► TestCreateSuccess()      ─► TearDownTest()
   ├─ SetupTest() ─► TestCreateNilInfo()      ─► TearDownTest()
   ├─ SetupTest() ─► TestCreateServiceError() ─► TearDownTest()
   └─ ...
```

`SetupTest` вызывается **перед каждым** тестовым методом, `TearDownTest` — после. Тесты сами
становятся методами структуры, поэтому общее состояние (контекст, мок, тестируемый объект)
лежит в полях, а не тащится через аргументы:

```go
type APISuite struct {
	suite.Suite

	ctx        context.Context
	ufoService *mocks.UFOService
	api        *api
}

func (s *APISuite) SetupTest() {
	s.ctx = context.Background()
	s.ufoService = mocks.NewUFOService(s.T())
	s.api = NewAPI(s.ufoService)
}
```

Ключевое: мок **пересоздаётся** в `SetupTest`. Не «очищается» — создаётся заново. Ожидания
предыдущего теста физически не могут протечь в следующий.

### 8.2. TearDownTest и AssertExpectations

```go
func (s *APISuite) TearDownTest() {
	s.T().Log("TearDownTest: очистка после", s.T().Name())
	s.ufoService.AssertExpectations(s.T())
}
```

`AssertExpectations` проверяет, что все настроенные `EXPECT()` действительно сработали.
Сценарий, от которого это спасает: настроили `EXPECT().Get(...)`, но код из-за бага ушёл в
другую ветку и до `Get` не дошёл. Проверки результата случайно совпали → тест зелёный →
баг уехал в прод.

В mockery v3 этот вызов **избыточен**: `mocks.NewUFOService(s.T())` уже регистрирует проверку
через `t.Cleanup()`. В примере он оставлен намеренно — показать, что `TearDownTest` это в
принципе то место, где живут пост-условия.

### 8.3. Где `Require`, а где `Assert`

Тонкость testify/suite: `s.NoError(err)` — это **Assert**-режим (эквивалент
`s.Assert().NoError(err)`). Для Require нужно писать явно: `s.Require().NoError(err)`.

В примере два слоя намеренно написаны по-разному:

```go
// API-слой: Require, потому что дальше идёт обращение к полям ответа
res, err := s.api.Create(s.ctx, req)
s.Require().NoError(err)
s.Require().NotNil(res)
s.Require().Equal(expectedUUID, res.GetUuid())   // при nil res здесь была бы паника
```

```go
// Сервисный слой: Assert, проверки независимы
uuid, err := s.service.Create(s.ctx, createInput)
s.Error(err)
s.ErrorIs(err, repoErr)
s.Empty(uuid)                                     // хотим увидеть обе проблемы сразу
```

Правило то же, что в разделе 3, просто применённое к разной форме кода: если следующая строка
разыменовывает результат — ставь `Require`.

### 8.4. Проверка gRPC-кода

```go
func (s *APISuite) TestCreateNilInfo() {
	req := &ufov1.CreateRequest{Location: ""}

	res, err := s.api.Create(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}
```

Заметь: мок здесь **не настроен вообще**. Это и есть проверка «валидация отработала до вызова
сервиса» — если бы API всё-таки дёрнул `Create`, мок упал бы на неожиданном вызове.

### 8.5. `mock.MatchedBy` — когда точное сравнение невозможно

Сервисный `Create` генерирует `Uuid` и `CreatedAt` внутри себя:

```go
sighting := model.Sighting{
	Uuid:      uuid.NewString(),
	CreatedAt: time.Now(),
	...
}
sighting.Credibility = calculateCredibility(sighting)
```

Значит, написать `EXPECT().Create(ctx, expectedSighting)` нельзя — заранее эти два поля
неизвестны. Решение — матчер-предикат:

```go
s.ufoRepository.EXPECT().Create(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
	return matchesInput(createInput)(sg) && sg.Credibility == "high"
})).Return(nil)
```

где `matchesInput` сверяет то, что проверить можно:

```go
func matchesInput(in input.CreateSightingInput) func(s model.Sighting) bool {
	return func(s model.Sighting) bool {
		return s.Uuid != "" &&                                  // сгенерирован — не пуст
			!s.CreatedAt.IsZero() &&                            // проставлен — не нулевой
			equalTimePtr(s.ObservedAt, in.ObservedAt) &&        // сравнение по значению,
			s.Location == in.Location &&                        // а не по адресу указателя
			s.Description == in.Description &&
			equalStringPtr(s.Color, in.Color) &&
			equalBoolPtr(s.Sound, in.Sound) &&
			equalInt32Ptr(s.DurationSeconds, in.DurationSeconds)
	}
}
```

Два приёма, которые стоит забрать себе:

- **runtime-значения проверяем на свойство, а не на равенство**: `Uuid != ""`,
  `!CreatedAt.IsZero()`;
- **указатели сравниваем по значению**: `in` и `sighting` — разные экземпляры, `*string` в них
  указывают на разные адреса, хотя строки одинаковые. Отсюда пачка хелперов `equalXxxPtr`.

## 9. `7_parallel_tests` — параллельные table-driven

Тот же сервис, те же слои, те же моки — другая **форма** тестов. Suite заменён на standalone
функции с таблицей и `t.Parallel()`.

### 9.1. Два уровня параллельности

```go
func TestCreate(t *testing.T) {
	t.Parallel()                    // (1) TestCreate ‖ TestGet ‖ TestUpdate ‖ TestDelete
	...
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()            // (2) подтесты параллельно друг другу
			...
		})
	}
}
```

`t.Parallel()` на верхнем уровне разрешает `TestCreate` идти одновременно с другими top-level
тестами пакета. `t.Parallel()` внутри `t.Run` — одновременно идут кейсы одной таблицы.

### 9.2. Главное правило: мок создаётся внутри подтеста

```go
t.Run(tc.name, func(t *testing.T) {
	t.Parallel()

	// Каждый подтест создаёт СВОЙ мок — изоляция, без data race
	ufoService := mocks.NewUFOService(t)
	tc.setupMock(ufoService)

	a := NewAPI(ufoService)
	res, err := a.Create(ctx, tc.args.req)
	...
})
```

Если бы мок был общим (как поле suite), параллельные подтесты одновременно писали и читали
`mock.ExpectedCalls` → data race и плавающие падения. Именно поэтому suite и `t.Parallel()`
плохо совмещаются: у suite общее состояние по определению.

Заодно `mocks.NewUFOService(t)` получает **`t` подтеста**, а не родительский — значит и
автоматическая проверка ожиданий через `t.Cleanup` привяжется к правильному тесту.

### 9.3. Форма кейса: два колбэка

```go
tests := []struct {
	name      string
	args      args
	setupMock func(svc *mocks.UFOService)
	expected  expected
}{
	{
		name: "успешное создание наблюдения",
		args: args{req: req},
		setupMock: func(svc *mocks.UFOService) {
			svc.EXPECT().Create(ctx, expectedModelInfo).Return(expectedUUID, nil)
		},
		expected: expected{
			res: &ufov1.CreateResponse{Uuid: expectedUUID},
			errCheck: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	},
	{
		name: "пустой location — codes.InvalidArgument",
		args: args{req: &ufov1.CreateRequest{Location: ""}},
		setupMock: func(_ *mocks.UFOService) {
			// Мок не настраиваем — валидация до вызова сервиса
		},
		expected: expected{
			res: nil,
			errCheck: func(t *testing.T, err error) {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, codes.InvalidArgument, st.Code())
			},
		},
	},
	...
}
```

- `setupMock` — кейс сам решает, как ведёт себя зависимость (и ведёт ли себя вообще);
- `errCheck` — кейс сам решает, **как** проверять ошибку. Универсального `expectedErr error`
  не хватает: где-то нужен `NoError`, где-то gRPC-код через `status.FromError`, где-то
  `ErrorIs`. Колбэк снимает эту проблему без ветвлений в теле цикла.

Вложенные типы `args` и `expected` — не обязательны, но заметно улучшают читаемость таблицы,
когда полей становится больше трёх-четырёх.

### 9.4. `tc := tc` больше не нужен

До Go 1.22 переменная цикла `for range` переиспользовалась между итерациями. Замыкания
параллельных подтестов читали её **после** окончания цикла и все видели последний элемент —
классическая ловушка, из-за которой писали `tc := tc` перед `t.Run`.

Начиная с Go 1.22 каждая итерация создаёт свою переменную. Захват не нужен, в примере он убран.
Модули недели объявляют `go 1.26.0`, так что это уже безопасно по умолчанию.

### 9.5. Suite или parallel

| Аспект | Suite (пример 6) | Parallel table-driven (пример 7) |
|--------|------------------|----------------------------------|
| Структура теста | Методы на struct | Отдельные функции |
| Общая подготовка | `SetupTest` — один раз описана | Повторяется в каждом `t.Run` |
| Мок | Один на тест, поле структуры | Свой в каждом подтесте |
| Параллельность | Практически невозможна | Штатный режим |
| Добавить кейс | Новый метод `Test*` | Новая строка в таблице |
| Читаемость при 10+ кейсах | Файл растёт вширь | Таблица остаётся компактной |

Suite удобен, когда подготовка тяжёлая и одинаковая, а кейсов немного и они разнородные.
Parallel table-driven — когда кейсов много, они однотипны и хочется скорости. В курсе показаны
оба, потому что в реальных проектах встречаются оба.

### 9.6. Почему здесь `-race`

```yaml
test:
  desc: "Запускает unit-тесты с race-детектором"
  cmds:
    - go test -v -race -count=1 ./...
```

- `-race` — единственный способ поймать гонку, которую даёт неаккуратная параллельность
  (например, общий мок). Без него тест может годами быть зелёным и падать раз в сто прогонов;
- `-count=1` — отключает кэш результатов `go test`, чтобы «зелёный» не оказался вчерашним.

## 10. E2E через bufconn

Юнит-тесты покрыли API и сервис. Остались репозиторий, конвертеры, gRPC-сериализация и
проводка DI — всё то, что видно только в сборе. Для этого — `tests/api_test.go`
(есть в `layers/`, в примерах 6 и 7).

### 10.1. Что такое bufconn

`google.golang.org/grpc/test/bufconn` — listener поверх буфера в памяти. gRPC-сервер и клиент
работают штатно (кодирование protobuf, HTTP/2-фреймы, статусы), но байты не уходят в сетевой
стек. Нет портов — нет конфликтов при параллельном запуске, нет «занят 50051», нет ожидания
подъёма сервера.

### 10.2. Разбор `TestMain`

```go
const bufSize = 1024 * 1024

var (
	lis    *bufconn.Listener
	client ufov1.UFOServiceClient
)

func TestMain(m *testing.M) {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()

	// DI: собираем сервис как в main.go
	repo := ufoRepository.NewRepository()
	weatherClient := weatherStub.New()
	service := ufoService.NewService(repo, weatherClient)
	api := ufov1API.NewAPI(service)

	ufov1.RegisterUFOServiceServer(s, api)

	go func() {
		if err := s.Serve(lis); err != nil {
			panic(err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	...
	client = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	conn.Close()
	s.Stop()
	os.Exit(code)
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}
```

По шагам:

| Шаг | Что происходит |
|-----|----------------|
| `bufconn.Listen(1MB)` | in-memory listener вместо `net.Listen("tcp", ...)` |
| DI-цепочка | **та же, что в `main.go`** — тест проверяет реальную сборку, а не тестовую |
| `RegisterUFOServiceServer` | тот же вызов, что в проде |
| `go s.Serve(lis)` | сервер в отдельной горутине |
| `passthrough:///bufnet` | схема резолвера, отключающая разбор адреса — реальный коннект даёт `WithContextDialer` |
| `WithContextDialer(bufDialer)` | вместо TCP-дозвона отдаём соединение из буфера |
| `insecure.NewCredentials()` | TLS в памяти не нужен |
| `m.Run()` | запуск всех `TestXxx` пакета |
| `conn.Close()` / `s.Stop()` | обязательная уборка до `os.Exit` — `defer` после `os.Exit` не сработает |

Клиент и listener — пакетные переменные, поэтому каждый тест просто пишет `client.Create(...)`.

### 10.3. Хелперы

```go
func assertGRPCStatus(t *testing.T, err error, expectedCode codes.Code) {
	t.Helper()
	...
	st, ok := status.FromError(err)
	assert.True(t, ok, "ошибка должна быть gRPC статусом")
	assert.Equal(t, expectedCode, st.Code(), "неверный gRPC код: %s", st.Message())
}

func createTestSighting(t *testing.T, client ufov1.UFOServiceClient, location, description string) string {
	t.Helper()
	resp, err := client.Create(...)
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())
	return resp.GetUuid()
}
```

`t.Helper()` — важная мелочь: без него при падении Go покажет строку внутри хелпера, а с ним —
строку вызова в самом тесте. В хелперах, которые вызываются из десятка мест, это разница между
«понятно, что упало» и «непонятно, что упало».

`createTestSighting` решает вторую типовую задачу E2E: тесту `Get` нужна уже существующая
запись. Хранилище общее на весь пакет, поэтому каждый тест создаёт свои данные сам, а не
полагается на порядок выполнения.

### 10.4. Почему репозиторий не покрывают юнит-тестами

Он проверяется здесь, в связке `Create → Get → Update → Delete`. Юнит-тест репозитория
пришлось бы писать против `map` под мьютексом — то есть повторять реализацию в ассертах. Такой
тест не ловит баги, но ломается при любом рефакторинге. E2E-прогон даёт больше: он заодно
проверяет оба конвертера, soft delete и то, что коды ошибок доезжают до клиента.

---

# Часть III. Инструменты и шпаргалка

## 11. Инструментарий

### 11.1. Taskfile

В каждом примере — свой `Taskfile.yaml` с закреплёнными версиями тулинга:

```yaml
vars:
  GO_VERSION: '1.26.0'
  GOLANGCI_LINT_VERSION: 'v2.12.2'
  MOCKERY_VERSION: 'v3.7.0'
  BIN_DIR: '{{.ROOT_DIR}}/bin'
```

Инструменты ставятся **локально в `bin/`** (`GOBIN={{.BIN_DIR}} go install ...`), а не глобально:
у всех одинаковые версии, `bin/` в `.gitignore`, блок `status:` не даёт переустанавливать
уже стоящее.

| Команда | Что делает |
|---------|------------|
| `task setup` | ставит gofumpt, gci, golangci-lint, mockery |
| `task format` | `gofumpt -extra` + `gci` (сортировка импортов на три группы), пропуская `mocks/` и `pkg/proto/` |
| `task lint` | golangci-lint по `.golangci.yml` |
| `task mocks:gen` | генерация моков |
| `task test` | юнит-тесты (в примере 7 — с `-race -count=1`) |
| `task test:api` | E2E через bufconn: `go test -v ./tests/...` |
| `task test:coverage` | покрытие с порогом |
| `task run` | в `layers/` — поднять gRPC-сервер на `:50051` |

Сгенерированный код форматтеры не трогают намеренно: `mocks/` и `pkg/proto/` перезаписываются
генераторами, форматировать их бессмысленно.

### 11.2. mockery v3

Что нужно помнить на практике:

- конфиг — `.mockery.yaml` в корне примера; интерфейсы отбираются регекспом
  (`include-interface-regex: ".*Service"`), поэтому важно единообразно называть интерфейсы;
- в примере 6 указаны **два пакета** — API-слой (`.*Service`) и сервисный (`.*Repository`);
- моки кладутся рядом с интерфейсом: `internal/api/ufo/v1/mocks/mock_ufo_service.go`;
- файлы моков **коммитятся** — иначе чистый клон не соберётся без `task mocks:gen`;
- меняешь интерфейс → перегенерируй. Забыл — увидишь ошибку компиляции теста, что лучше, чем
  молчаливое расхождение;
- `mocks.NewXxx(t)` всегда получает `*testing.T` — на этом держится автопроверка ожиданий.

### 11.3. gofakeit

`gofakeit.City()`, `gofakeit.UUID()`, `gofakeit.Error()`, `gofakeit.LoremIpsumParagraph(...)`
в тестах говорят: «значение здесь роли не играет». Это защищает от ложной связи между тестом
и данными.

Где так делать **не** надо:

- когда значение участвует в логике (граница диапазона, спецсимвол, пустая строка) — пиши
  явную константу, иначе тест перестанет проверять то, ради чего написан;
- когда данные должны быть воспроизводимы для отладки — случайность превратит падение в
  «у меня не воспроизводится».

### 11.4. Покрытие

```yaml
test:coverage:
  vars:
    TARGET_PACKAGES:
      sh: go list ./internal/... | grep -v "/mocks" | grep -E '/(api|service)/' | tr '\n' ','
  cmds:
    - go test -v -race -covermode=atomic -coverprofile={{.COVERAGE_FILE}} -coverpkg={{.TARGET_PACKAGES}} ./...
    - go tool cover -func={{.COVERAGE_FILE}}
    - task: coverage:threshold
```

Считается покрытие только `api` и `service` — то есть **бизнес-логики**. Из подсчёта выброшены
`mocks/` (сгенерированный код) и всё, что вне этих двух слоёв. Это честнее общей цифры: 90%
покрытия, набранных на геттерах и сгенерированных файлах, ничего не говорят о качестве тестов.

`coverage:threshold` сравнивает итог с порогом и валит задачу, если ниже, — готовый кирпич
для CI.

---

## 12. Шпаргалка

### Что чем тестировать

| Тестируем | Инструмент | Пример |
|-----------|-----------|--------|
| Чистую функцию (нет зависимостей) | Table-driven + testify | 2, 3 |
| Функцию с зависимостью, важен только **результат** зависимости | Стаб | 4 |
| Функцию с зависимостью, важны **аргументы и факт вызова** | Мок (mockery) | 5 |
| Слой с тяжёлой одинаковой подготовкой, кейсы разнородные | `testify/suite` | 6 |
| Слой, кейсов много и они однотипны | Parallel table-driven | 7 |
| Связку слоёв, сериализацию, DI | bufconn E2E | `layers/tests/` |
| Хранилище / адаптер к инфраструктуре | Только E2E | — |

### Чек-лист перед тем, как писать тест

1. **Есть ли что мокать?** Если функция чистая — просто таблица, никаких двойников.
2. **Зависимость приходит через интерфейс?** Если через конкретный тип — сначала интерфейс,
   потом тест. Интерфейс объявляет тот, кто его использует (`deps.go`).
3. **Стаб или мок?** Нужна проверка взаимодействия → мок. Нужен только ответ → стаб.
4. **Suite или parallel?** Много однотипных кейсов → parallel. Тяжёлая общая подготовка → suite.
5. **Где `require`, где `assert`?** Если следующая строка разыменовывает результат — `require`.
6. **Runtime-значения (UUID, время) в ожиданиях?** → `mock.MatchedBy` с проверкой свойств.
7. **Кейс «зависимость вернула ошибку» есть?** Проверь `ErrorIs`, а не текст.
8. **Кейс «до зависимости не дошли» есть?** Валидация должна отсекать вызов — мок не настраивается.

### Типовые ошибки

| Ошибка | Чем кончается |
|--------|---------------|
| Общий мок + `t.Parallel()` в подтестах | Data race на `mock.ExpectedCalls`, плавающие падения |
| `assert` там, где нужен `require` | Паника по nil-указателю вместо внятного сообщения |
| Мок без проверки ожиданий | Зелёный тест при неработающем коде |
| Сравнение ошибок по тексту вместо `ErrorIs` | Тест ломается от правки формулировки |
| `%v` вместо `%w` при оборачивании ошибки | `errors.Is` перестаёт находить причину — тихо |
| Ассерт на конкретное значение из `gofakeit` | Тест падает случайным образом |
| Тест собирает `input` руками вместо конвертера | Ломается при каждом новом поле в контракте |
| Юнит-тест на in-memory репозиторий | Переписывание реализации в ассертах |
| `go test` без `-count=1` в CI | Зелёный результат из кэша |

---

## Что читать дальше

- README каждого примера — короткая выжимка «что смотреть в файлах»:
  [1](unit_tests/1_easy_test/README.md) ·
  [2](unit_tests/2_common_unit_test/README.md) ·
  [3](unit_tests/3_testify_unit_test/README.md) ·
  [4](unit_tests/4_unit_test_with_stubs/README.md) ·
  [5](unit_tests/5_unit_test_with_mocks/README.md) ·
  [6](unit_tests/6_unit_test_in_clean_arch/README.md) ·
  [7](unit_tests/7_parallel_tests/README.md) ·
  [layers](layers/README.md)
- [`../week_1/HOW_IT_WORKS.md`](../week_1/HOW_IT_WORKS.md) — откуда взялся proto-контракт и
  сгенерированный код, который здесь используется как данность
- [`../week_1/GRPC_CONNECTIONS.md`](../week_1/GRPC_CONNECTIONS.md) — keepalive-параметры из
  `layers/cmd/grpc_server/main.go`
