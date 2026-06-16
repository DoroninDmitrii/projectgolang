# Параллельные table-driven тесты

Тот же UFO-сервис (архитектура как в [`layers`](../../layers/)), но вместо `testify/suite` — параллельные table-driven тесты. Акцент на параллельность, изоляцию моков и удобство добавления кейсов.

## Отличия от suite-подхода

| Аспект | [Пример 6](../6_unit_test_in_clean_arch/) (suite) | Этот пример (parallel) |
|--------|---------------------|----------------------|
| Структура | Методы на suite struct | Standalone функции |
| Мок | Один на тест (`SetupTest`) | Свой на каждый подтест |
| Параллельность | Нет | `t.Parallel()` на двух уровнях |
| Кейсы | Отдельные методы | Таблица `[]struct{...}` |

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Два уровня `t.Parallel()` | `internal/api/ufo/v1/create_test.go` | Top-level тесты параллельно друг с другом + подтесты параллельно внутри |
| Мок внутри подтеста | там же | `mocks.NewUFOService(t)` создаётся в каждом `t.Run` — иначе data race |
| `setupMock` callback | там же | Каждый кейс в таблице настраивает мок через `func(svc *mocks.UFOService)` |
| `errCheck` callback | там же | Разные кейсы требуют разной проверки: `NoError`, gRPC-код, `ErrorIs` |
| `mock.MatchedBy` | `internal/service/ufo/create_test.go` | Для полей, генерируемых в runtime (UUID, timestamps) — матчер вместо точного сравнения |

## tc := tc — уже не нужен

С Go 1.22 каждая итерация `for range` создаёт свою переменную. Захват `tc := tc` перед `t.Run` больше не требуется.

## Как запустить

```bash
task mocks:gen         # Генерация моков
task test              # Запуск с -race
task test:coverage     # Тесты с покрытием
```

## Структура тестовых файлов

```
internal/
├── api/ufo/v1/
│   ├── create_test.go          TestCreate (table-driven, parallel)
│   ├── get_test.go             TestGet
│   ├── update_test.go          TestUpdate
│   ├── delete_test.go          TestDelete
│   └── mocks/                  сгенерированный мок UFOService
├── service/ufo/
│   ├── create_test.go          TestCreate
│   ├── get_test.go             TestGet
│   └── mocks/                  сгенерированный мок UFORepository
```
