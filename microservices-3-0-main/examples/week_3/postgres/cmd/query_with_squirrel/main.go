package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const (
	// Имя таблицы
	tableNote = "note"

	// Имена колонок таблицы note
	columnID        = "id"
	columnTitle     = "title"
	columnBody      = "body"
	columnCreatedAt = "created_at"
	columnUpdatedAt = "updated_at"
)

//nolint:funlen // Учебный пример — все SQL-операции показаны последовательно в одной функции
func main() {
	ctx := context.Background()

	err := godotenv.Load(".env")
	if err != nil {
		slog.Error("не удалось загрузить .env файл", "error", err)
		return
	}

	dbURI := os.Getenv("DB_URI")

	// Создаём пул соединений с базой данных
	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к базе данных", "error", err)
		return
	}
	defer pool.Close()

	// Делаем запрос на вставку записи в таблицу note
	builderInsert := sq.Insert(tableNote).
		PlaceholderFormat(sq.Dollar).
		Columns(columnTitle, columnBody).
		Values(gofakeit.City(), gofakeit.Address().Street).
		Suffix("RETURNING " + columnID)

	query, args, err := builderInsert.ToSql()
	if err != nil {
		slog.Error("не удалось построить запрос", "error", err)
		return
	}

	var noteID int
	err = pool.QueryRow(ctx, query, args...).Scan(&noteID)
	if err != nil {
		slog.Error("не удалось вставить запись", "error", err)
		return
	}

	slog.Info("вставлена запись", "id", noteID)

	// Делаем запрос на выборку записей из таблицы note
	builderSelect := sq.Select(columnID, columnTitle, columnBody, columnCreatedAt, columnUpdatedAt).
		From(tableNote).
		PlaceholderFormat(sq.Dollar).
		OrderBy(columnID + " ASC").
		Limit(10)

	query, args, err = builderSelect.ToSql()
	if err != nil {
		slog.Error("не удалось построить запрос", "error", err)
		return
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		slog.Error("не удалось выполнить выборку", "error", err)
		return
	}
	defer rows.Close()

	var id int
	var title, body string
	var createdAt time.Time
	var updatedAt sql.NullTime

	for rows.Next() {
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

	// Делаем запрос на обновление записи в таблице note
	builderUpdate := sq.Update(tableNote).
		PlaceholderFormat(sq.Dollar).
		Set(columnTitle, gofakeit.City()).
		Set(columnBody, gofakeit.Address().Street).
		Set(columnUpdatedAt, time.Now()).
		Where(sq.Eq{columnID: noteID})

	query, args, err = builderUpdate.ToSql()
	if err != nil {
		slog.Error("не удалось построить запрос", "error", err)
		return
	}

	res, err := pool.Exec(ctx, query, args...)
	if err != nil {
		slog.Error("не удалось обновить запись", "error", err)
		return
	}

	slog.Info("обновлено строк", "count", res.RowsAffected())

	// Делаем запрос на получение изменённой записи из таблицы note
	builderSelectOne := sq.Select(columnID, columnTitle, columnBody, columnCreatedAt, columnUpdatedAt).
		From(tableNote).
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{columnID: noteID}).
		Limit(1)

	query, args, err = builderSelectOne.ToSql()
	if err != nil {
		slog.Error("не удалось построить запрос", "error", err)
		return
	}

	err = pool.QueryRow(ctx, query, args...).Scan(&id, &title, &body, &createdAt, &updatedAt)
	if err != nil {
		slog.Error("не удалось выполнить выборку", "error", err)
		return
	}

	slog.Info("запись", "id", id, "title", title, "body", body, "created_at", createdAt, "updated_at", updatedAt)

	// Создаём ещё одну запись для демонстрации удаления
	builderInsertForDelete := sq.Insert(tableNote).
		PlaceholderFormat(sq.Dollar).
		Columns(columnTitle, columnBody).
		Values(gofakeit.City(), gofakeit.Address().Street).
		Suffix("RETURNING " + columnID)

	query, args, err = builderInsertForDelete.ToSql()
	if err != nil {
		slog.Error("не удалось построить запрос", "error", err)
		return
	}

	var deleteNoteID int
	err = pool.QueryRow(ctx, query, args...).Scan(&deleteNoteID)
	if err != nil {
		slog.Error("не удалось вставить запись для удаления", "error", err)
		return
	}

	slog.Info("вставлена запись для удаления", "id", deleteNoteID)

	// Делаем запрос на удаление записи из таблицы note
	builderDelete := sq.Delete(tableNote).
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{columnID: deleteNoteID})

	query, args, err = builderDelete.ToSql()
	if err != nil {
		slog.Error("не удалось построить запрос на удаление", "error", err)
		return
	}

	res, err = pool.Exec(ctx, query, args...)
	if err != nil {
		slog.Error("не удалось удалить запись", "error", err)
		return
	}

	slog.Info("удалено строк", "count", res.RowsAffected(), "id", deleteNoteID)
}
