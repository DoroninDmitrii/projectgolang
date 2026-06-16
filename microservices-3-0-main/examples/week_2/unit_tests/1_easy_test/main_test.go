package main

import "testing"

// TestIsStupid — правильная версия теста с использованием пакета testing
// Сравните с функцией testIsStupid() в main.go, которая написана «вручную»
func TestIsStupid(t *testing.T) {
	// Нет мозга — значит тупой
	if !isStupid(false) {
		t.Error("ожидали true для hasBrain=false")
	}

	// Есть мозг — значит не тупой
	if isStupid(true) {
		t.Error("ожидали false для hasBrain=true")
	}
}
