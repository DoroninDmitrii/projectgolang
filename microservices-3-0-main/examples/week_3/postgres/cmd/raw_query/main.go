package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	err := godotenv.Load(".env")
	if err != nil {
		slog.Error("не удалось загрузить .env файл", "error", err)
		return
	}

	dbURI := os.Getenv("DB_URI")

	// Создаём соединение с базой данных
	con, err := pgx.Connect(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к базе данных", "error", err)
		return
	}
	defer func() {
		cerr := con.Close(ctx)
		if cerr != nil {
			slog.Error("не удалось закрыть соединение", "error", cerr)
		}
	}()

	// Проверяем, что соединение с базой установлено
	err = con.Ping(ctx)
	if err != nil {
		slog.Error("база данных недоступна", "error", err)
		return
	}

	// Делаем запрос на вставку записи в таблицу note
	res, err := con.Exec(ctx, "INSERT INTO note (title, body) VALUES ($1, $2)", gofakeit.City(), gofakeit.Address().Street)
	if err != nil {
		slog.Error("не удалось вставить запись", "error", err)
		return
	}

	slog.Info("вставлено строк", "count", res.RowsAffected())

	// Делаем запрос на выборку записей из таблицы note
	rows, err := con.Query(ctx, "SELECT id, title, body, created_at, updated_at FROM note")
	if err != nil {
		slog.Error("не удалось выполнить выборку", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var title, body string
		var createdAt time.Time
		var updatedAt sql.NullTime

		err = rows.Scan(&id, &title, &body, &createdAt, &updatedAt)
		if err != nil {
			slog.Error("не удалось прочитать строку", "error", err)
			return
		}

		slog.Info("запись", "id", id, "title", title, "body", body, "created_at", createdAt, "updated_at", updatedAt)
	}

	if err = rows.Err(); err != nil {
		slog.Error("ошибка при итерации по строкам", "error", err)
		return
	}
}
