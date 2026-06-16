package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// RequestLogger создает middleware для логирования времени выполнения запросов
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Засекаем время начала обработки запроса
		startTime := time.Now()

		// Логируем начало запроса
		slog.Info("начало запроса", "method", r.Method, "path", r.URL.Path)

		// Передаем управление следующему обработчику
		next.ServeHTTP(w, r)

		// Вычисляем время выполнения запроса
		duration := time.Since(startTime)

		// Логируем окончание запроса с временем выполнения
		slog.Info("запрос завершен", "method", r.Method, "path", r.URL.Path, "duration", duration)
	})
}
