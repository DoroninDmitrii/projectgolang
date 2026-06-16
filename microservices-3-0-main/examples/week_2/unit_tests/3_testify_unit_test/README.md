# Тесты с testify (assert/require)

Тот же расчёт кредитного рейтинга, но функция теперь возвращает `(int, error)`, а тесты используют `testify/assert` и `testify/require`.

## Концепция

**testify** — стандартная библиотека для тестирования в Go-проектах. Два ключевых пакета:

| Пакет | При провале | Для чего |
|-------|-------------|----------|
| `require` | `t.FailNow()` — тест останавливается | Предусловия: если дальше будет panic |
| `assert` | `t.Fail()` — тест продолжается | Проверки значений: собрать все расхождения |

**Правило: require для "ворот" (gates), assert для "проверок" (checks).**

```go
result, err := CalculateCreditScore(client)
require.NoError(t, err)          // gate: если err != nil, дальше бессмысленно
assert.Equal(t, expected, result) // check: сравнение значения
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| require vs assert | `credit_score/calculate_credit_score_test.go` | `require.NoError` перед `assert.Equal` — правильный порядок |
| Тесты ошибок | там же, кейсы "некорректный возраст/зарплата" | `require.ErrorIs(t, err, ErrInvalidAge)` — проверка типа ошибки |
| Валидация в функции | `credit_score/calculate_credit_score.go` | Добавлены `ErrInvalidAge`, `ErrInvalidSalary` — функция возвращает ошибки |

Сравни с [примером 2](../2_common_unit_test/) — та же логика, но без обработки ошибок и с ручными `t.Errorf`.

## Как запустить

```bash
go test ./...
```
