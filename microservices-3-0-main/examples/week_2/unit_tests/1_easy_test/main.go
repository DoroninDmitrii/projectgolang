package main

import (
	"log/slog"

	"github.com/brianvoe/gofakeit/v7"
)

func main() {
	hasBrain := gofakeit.Bool()

	if isStupid(hasBrain) {
		slog.Info("Да, ты тупой")
		return
	}

	slog.Info("Нет, ты не тупой")
}

// isStupid возвращает true, если у человека нет мозга
func isStupid(hasBrain bool) bool {
	return !hasBrain
}

// testIsStupid — пример «ручного» теста без использования пакета testing
// Минусы: функция нигде не вызывается, нет интеграции с go test, нет удобных ассертов
// Смотрите main_test.go для правильной версии этого же теста
//
//nolint:unused // намеренно не вызывается — это антипаттерн для демонстрации
func testIsStupid() {
	slog.Info("тест isStupid(false) — нет мозга, значит тупой")
	if !isStupid(false) {
		slog.Error("тест провален: ожидали true")
		return
	}

	slog.Info("тест пройден")

	slog.Info("тест isStupid(true) — есть мозг, значит не тупой")
	if isStupid(true) {
		slog.Error("тест провален: ожидали false")
		return
	}

	slog.Info("тест пройден")
}
