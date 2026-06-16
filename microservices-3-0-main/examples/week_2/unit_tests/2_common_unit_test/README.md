# Table-driven тесты

Расчёт кредитного рейтинга клиента, покрытый table-driven тестами на стандартном `testing`.

## Концепция

Table-driven tests — стандартный паттерн в Go: массив тест-кейсов с входными данными и ожидаемым результатом, прогоняемый в цикле через `t.Run()`. Каждый кейс — именованный подтест, видимый в выводе `go test -v`.

```go
tests := []struct {
    name     string
    client   Client
    expected int
}{...}

for _, test := range tests {
    t.Run(test.name, func(t *testing.T) {
        result := CalculateCreditScore(test.client)
        if result != test.expected {
            t.Errorf(...)
        }
    })
}
```

## На что обратить внимание

| Что | Файл | Зачем |
|-----|------|-------|
| Table-driven паттерн | `credit_score/calculate_credit_score_test.go` | Структура `[]struct{name, input, expected}` + цикл с `t.Run` |
| Ручные ассерты | там же | `t.Errorf()` вместо testify — показывает, как это выглядит без библиотек |
| Чистая функция | `credit_score/calculate_credit_score.go` | Нет побочных эффектов, нет зависимостей — идеальный кандидат для unit-тестов |

## Как запустить

```bash
go test ./...
```
