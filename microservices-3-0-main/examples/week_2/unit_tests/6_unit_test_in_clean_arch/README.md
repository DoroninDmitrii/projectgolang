# Юнит-тесты в чистой архитектуре (testify/suite)

Юнит-тестирование API- и сервисного слоёв UFO-сервиса с использованием `testify/suite`. Архитектура аналогична примеру [`layers`](../../layers/). Акцент — на suite-подходе к организации тестов.

## Концепция

**testify/suite** — структурированный подход к тестам через struct с методами:

- `SetupTest()` — вызывается перед каждым тестовым методом. Создаёт свежий мок — никаких остатков от предыдущих тестов.
- `TearDownTest()` — вызывается после каждого метода. `AssertExpectations` проверяет, что все ожидания мока выполнены.
- `Test*()` методы — тест-кейсы.

```
suite.Run(t, new(APISuite))
    -> SetupTest()  -> TestCreateSuccess()  -> TearDownTest()
    -> SetupTest()  -> TestCreateNilInfo()   -> TearDownTest()
    -> ...
```

В mockery v3 `AssertExpectations` вызывается автоматически через `t.Cleanup()`, явный вызов в `TearDownTest` оставлен для наглядности.

## Что тестируем и что мокаем

| Слой | Мокаем | Проверяем |
|------|--------|-----------|
| API (`internal/api/ufo/v1/`) | `UFOService` | Валидация, конвертация proto <-> model, маппинг ошибок в gRPC-коды |
| Service (`internal/service/ufo/`) | `UFORepository` | Бизнес-логика, корректная обработка ошибок репозитория |

Репозиторный слой тестируется через E2E тесты (`tests/api_test.go`) с `bufconn`.

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Suite struct + setup/teardown | `internal/api/ufo/v1/suite_test.go` | Шаблон APISuite с `SetupTest`, `TearDownTest` |
| EXPECT API | `internal/api/ufo/v1/create_test.go` | `s.ufoService.EXPECT().Create(s.ctx, expectedModelInfo).Return(...)` |
| require vs assert в suite | там же | `s.Require().NoError(err)` vs `s.Equal(a, b)` (Assert по умолчанию) |
| gofakeit | там же | Случайные данные: `gofakeit.City()`, `gofakeit.UUID()` — конкретные значения не важны |
| Конфигурация mockery | `.mockery.yaml` | Два пакета: `.*Service` и `.*Repository` |
| E2E через bufconn | `tests/api_test.go` | Полный цикл Create -> Get -> Update -> Delete без сети |

## Как запустить

```bash
task mocks:gen         # Генерация моков
task test              # Юнит-тесты
task test:coverage     # Тесты с покрытием
```

## Структура тестовых файлов

```
internal/
├── api/ufo/v1/
│   ├── suite_test.go           APISuite (setup/teardown)
│   ├── create_test.go          TestCreateSuccess, TestCreateNilInfo, ...
│   ├── get_test.go             тесты Get
│   ├── update_test.go          тесты Update
│   ├── delete_test.go          тесты Delete
│   └── mocks/                  сгенерированный мок UFOService
├── service/ufo/
│   ├── suite_test.go           ServiceSuite
│   ├── create_test.go          тесты Create
│   └── mocks/                  сгенерированный мок UFORepository
```
