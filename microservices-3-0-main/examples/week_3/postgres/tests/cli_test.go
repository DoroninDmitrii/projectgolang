package tests

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	pool  *pgxpool.Pool
	dbURI string
)

// note представляет запись из таблицы note
type note struct {
	ID        int
	Title     string
	Body      string
	CreatedAt time.Time
	UpdatedAt sql.NullTime
}

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции и запускает тесты
func TestMain(m *testing.M) {
	// Загружаем переменные окружения из .env (креды для PostgreSQL)
	_ = godotenv.Load("../.env")

	pgUser := os.Getenv("POSTGRES_USER")
	pgPassword := os.Getenv("POSTGRES_PASSWORD")

	ctx := context.Background()

	// Поднимаем PostgreSQL в контейнере на случайном порту
	pgContainer, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase("postgres"),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	// Получаем строку подключения с динамическим портом
	dbURI, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// Накатываем миграции через goose
	sqlDB, err := sql.Open("pgx", dbURI)
	if err != nil {
		slog.Error("не удалось открыть sql.DB", "error", err)
		os.Exit(1)
	}

	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("не удалось установить диалект goose", "error", err)
		os.Exit(1)
	}
	if err = goose.Up(sqlDB, "../migrations"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}
	_ = sqlDB.Close()

	pool, err = pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	code := m.Run()

	pool.Close()

	if termErr := pgContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить контейнер", "error", termErr)
	}

	os.Exit(code)
}

// truncateNotes очищает таблицу note и сбрасывает sequence
func truncateNotes(t *testing.T) {
	t.Helper()

	_, err := pool.Exec(context.Background(), "TRUNCATE TABLE note RESTART IDENTITY")
	require.NoError(t, err, "не удалось очистить таблицу note")
}

// countNotes возвращает количество записей в таблице note
func countNotes(t *testing.T) int {
	t.Helper()

	var count int
	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM note").Scan(&count)
	require.NoError(t, err, "не удалось посчитать записи")

	return count
}

// getAllNotes возвращает все записи из таблицы note, отсортированные по id
func getAllNotes(t *testing.T) []note {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		"SELECT id, title, body, created_at, updated_at FROM note ORDER BY id")
	require.NoError(t, err, "не удалось выполнить выборку")
	defer rows.Close()

	var notes []note
	for rows.Next() {
		var n note
		err = rows.Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
		require.NoError(t, err, "не удалось прочитать строку")

		notes = append(notes, n)
	}

	return notes
}

// runProgram запускает Go программу из каталога проекта и возвращает вывод
// DB_URI передаётся через переменную окружения (testcontainers назначает случайный порт)
func runProgram(t *testing.T, program string) string {
	t.Helper()

	cmd := exec.Command("go", "run", fmt.Sprintf("./cmd/%s/main.go", program))
	cmd.Dir = ".." // запуск из postgres/
	cmd.Env = append(os.Environ(), "DB_URI="+dbURI)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "ошибка запуска %s:\n%s", program, string(output))

	return string(output)
}

// ============ raw_query ============.

// TestRawQuery_InsertsNote проверяет, что raw_query вставляет одну запись в БД
func TestRawQuery_InsertsNote(t *testing.T) {
	truncateNotes(t)

	output := runProgram(t, "raw_query")

	// Проверяем вывод программы
	assert.Contains(t, output, "вставлено строк", "в выводе должно быть сообщение о вставке")
	assert.Contains(t, output, "запись", "в выводе должны быть выбранные записи")

	// Проверяем состояние БД
	notes := getAllNotes(t)
	require.Len(t, notes, 1, "должна быть ровно 1 запись")

	assert.Equal(t, 1, notes[0].ID, "id первой записи должен быть 1")
	assert.NotEmpty(t, notes[0].Title, "title не должен быть пустым")
	assert.NotEmpty(t, notes[0].Body, "body не должен быть пустым")
	assert.False(t, notes[0].CreatedAt.IsZero(), "created_at должен быть заполнен")
	assert.False(t, notes[0].UpdatedAt.Valid, "updated_at должен быть NULL для новой записи")
}

// TestRawQuery_Idempotent проверяет, что каждый запуск добавляет новую запись
func TestRawQuery_Idempotent(t *testing.T) {
	truncateNotes(t)

	for i := range 3 {
		runProgram(t, "raw_query")
		assert.Equal(t, i+1, countNotes(t),
			"после запуска %d должно быть %d записей", i+1, i+1)
	}
}

// ============ query_with_squirrel ============.

// TestSquirrel_CRUDLifecycle проверяет полный цикл CRUD операций через squirrel
func TestSquirrel_CRUDLifecycle(t *testing.T) {
	truncateNotes(t)

	output := runProgram(t, "query_with_squirrel")

	// Проверяем вывод: все операции CRUD отражены в логах
	assert.Contains(t, output, "вставлена запись", "в выводе должна быть вставка")
	assert.Contains(t, output, "обновлено строк", "в выводе должно быть обновление")
	assert.Contains(t, output, "вставлена запись для удаления", "в выводе должна быть вставка для удаления")
	assert.Contains(t, output, "удалено строк", "в выводе должно быть удаление")

	// Проверяем состояние БД: осталась 1 запись (вторая удалена)
	notes := getAllNotes(t)
	require.Len(t, notes, 1, "должна остаться ровно 1 запись после удаления второй")

	// Оставшаяся запись должна быть обновлённой
	assert.NotEmpty(t, notes[0].Title, "title не должен быть пустым")
	assert.NotEmpty(t, notes[0].Body, "body не должен быть пустым")
	assert.True(t, notes[0].UpdatedAt.Valid, "updated_at должен быть заполнен после обновления")
}

// TestSquirrel_UpdateSetsTimestamp проверяет, что updated_at устанавливается корректно
func TestSquirrel_UpdateSetsTimestamp(t *testing.T) {
	truncateNotes(t)

	runProgram(t, "query_with_squirrel")

	notes := getAllNotes(t)
	require.Len(t, notes, 1)

	assert.True(t, notes[0].UpdatedAt.Valid, "updated_at должен быть заполнен")

	// Проверяем, что updated_at был установлен недавно (в пределах 1 минуты)
	diff := time.Since(notes[0].UpdatedAt.Time)
	assert.Less(t, diff, 1*time.Minute,
		"updated_at должен быть установлен недавно, разница: %v", diff)
}

// TestSquirrel_DeleteRemovesRecord проверяет, что DELETE действительно удаляет запись из БД
func TestSquirrel_DeleteRemovesRecord(t *testing.T) {
	truncateNotes(t)

	runProgram(t, "query_with_squirrel")

	// Программа вставляет 2 записи: первую обновляет, вторую удаляет
	// В итоге должна остаться только 1 запись
	assert.Equal(t, 1, countNotes(t), "удалённая запись не должна присутствовать в таблице")
}

// TestSquirrel_Idempotent проверяет, что каждый запуск оставляет +1 запись
func TestSquirrel_Idempotent(t *testing.T) {
	truncateNotes(t)

	for i := range 3 {
		runProgram(t, "query_with_squirrel")
		assert.Equal(t, i+1, countNotes(t),
			"после запуска %d должно быть %d записей", i+1, i+1)
	}
}
