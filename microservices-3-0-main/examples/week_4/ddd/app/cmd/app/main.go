package main

import (
	"context"
	"log/slog"

	"github.com/joho/godotenv"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/app"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/config"
)

func main() {
	// Загружаем переменные окружения из app.env (если файл существует)
	_ = godotenv.Load("app.env") //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	config.MustLoad()

	a := app.New(context.Background())

	if err := a.Run(); err != nil {
		slog.Error("ошибка при работе приложения", "error", err)
	}
}
