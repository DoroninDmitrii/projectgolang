package main

import (
	"context"
	"log/slog"

	"github.com/joho/godotenv"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/app"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/config"
)

func main() {
	// Загружаем переменные окружения из .env (если файл существует)
	_ = godotenv.Load("ufo.env") //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	config.MustLoad()

	a := app.New(context.Background())

	if err := a.Run(); err != nil {
		slog.Error("ошибка при работе приложения", "error", err)
	}
}
