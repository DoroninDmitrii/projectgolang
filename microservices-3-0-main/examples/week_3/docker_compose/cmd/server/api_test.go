package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	// pool пул соединений с БД для проверки состояния
	pool *pgxpool.Pool

	// ts in-memory HTTP сервер для тестов
	ts *httptest.Server
)

// user представляет запись из таблицы users
type user struct {
	ID        int
	Username  string
	Email     string
	CreatedAt time.Time
}

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции,
// запускает in-memory HTTP сервер и тесты
func TestMain(m *testing.M) {
	// Загружаем переменные окружения из .env (креды для PostgreSQL)
	_ = godotenv.Load("../../.env")

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
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// Накатываем миграции через goose
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		slog.Error("не удалось открыть sql.DB", "error", err)
		os.Exit(1)
	}

	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("не удалось установить диалект goose", "error", err)
		os.Exit(1)
	}
	if err = goose.Up(sqlDB, "../../migrations"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}
	_ = sqlDB.Close()

	// Подключаемся к БД
	pool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// Запускаем in-memory HTTP сервер через httptest
	ts = httptest.NewServer(setupHandler(pool))

	code := m.Run()

	ts.Close()
	pool.Close()

	if termErr := pgContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить контейнер", "error", termErr)
	}

	os.Exit(code)
}

// truncateUsers очищает таблицу users и сбрасывает sequence
func truncateUsers(t *testing.T) {
	t.Helper()

	_, err := pool.Exec(context.Background(), "TRUNCATE TABLE users RESTART IDENTITY")
	require.NoError(t, err, "не удалось очистить таблицу users")
}

// countUsers возвращает количество записей в таблице users
func countUsers(t *testing.T) int {
	t.Helper()

	var count int
	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&count)
	require.NoError(t, err, "не удалось посчитать записи")

	return count
}

// getUserByUsername находит пользователя по username
func getUserByUsername(t *testing.T, username string) *user {
	t.Helper()

	var u user
	err := pool.QueryRow(
		context.Background(),
		"SELECT id, username, email, created_at FROM users WHERE username = $1", username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt)
	if err != nil {
		return nil
	}

	return &u
}

// postUser отправляет POST-запрос на создание пользователя
func postUser(t *testing.T, username, email string) *http.Response {
	t.Helper()

	body := map[string]string{
		"username": username,
		"email":    email,
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := ts.Client().Post(
		ts.URL+"/users",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)

	return resp
}

// ============ CreateUser ============.

// TestCreateUser_Success проверяет успешное создание пользователя и его наличие в БД
func TestCreateUser_Success(t *testing.T) {
	truncateUsers(t)

	username := gofakeit.Username()
	email := gofakeit.Email()

	resp := postUser(t, username, email)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "ожидается статус 201")

	// Проверяем, что пользователь появился в БД
	u := getUserByUsername(t, username)
	require.NotNil(t, u, "пользователь должен быть в БД")
	assert.Equal(t, username, u.Username, "username должен совпадать")
	assert.Equal(t, email, u.Email, "email должен совпадать")
	assert.False(t, u.CreatedAt.IsZero(), "created_at должен быть заполнен")
}

// TestCreateUser_MissingUsername проверяет отказ при отсутствии username
func TestCreateUser_MissingUsername(t *testing.T) {
	body := map[string]string{
		"email": gofakeit.Email(),
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := ts.Client().Post(
		ts.URL+"/users",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "ожидается статус 400")
}

// TestCreateUser_MissingEmail проверяет отказ при отсутствии email
func TestCreateUser_MissingEmail(t *testing.T) {
	body := map[string]string{
		"username": gofakeit.Username(),
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := ts.Client().Post(
		ts.URL+"/users",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "ожидается статус 400")
}

// TestCreateUser_InvalidJSON проверяет отказ при невалидном JSON
func TestCreateUser_InvalidJSON(t *testing.T) {
	resp, err := ts.Client().Post(
		ts.URL+"/users",
		"application/json",
		bytes.NewReader([]byte("not a json")),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "ожидается статус 400")
}

// TestCreateUser_WrongMethod проверяет отказ при неправильном HTTP методе
func TestCreateUser_WrongMethod(t *testing.T) {
	resp, err := ts.Client().Get(ts.URL + "/users")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "ожидается статус 405")
}

// TestCreateUser_EmptyBody проверяет отказ при пустом теле запроса
func TestCreateUser_EmptyBody(t *testing.T) {
	resp, err := ts.Client().Post(
		ts.URL+"/users",
		"application/json",
		bytes.NewReader([]byte{}),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "ожидается статус 400")
}

// TestCreateUser_MultipleUsers проверяет создание нескольких пользователей и их наличие в БД
func TestCreateUser_MultipleUsers(t *testing.T) {
	truncateUsers(t)

	const usersCount = 5

	for i := range usersCount {
		resp := postUser(t, gofakeit.Username(), gofakeit.Email())
		resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "запрос %d должен вернуть 201", i+1)
	}

	// Проверяем, что все пользователи сохранились в БД
	assert.Equal(t, usersCount, countUsers(t),
		"в БД должно быть %d пользователей", usersCount)
}

// TestCreateUser_ValidationDoesNotWriteToDB проверяет, что невалидные запросы не создают записей
func TestCreateUser_ValidationDoesNotWriteToDB(t *testing.T) {
	truncateUsers(t)

	// Отправляем невалидные запросы
	invalidBodies := []string{
		`{"email": "test@test.com"}`,
		`{"username": "testuser"}`,
		`not a json`,
		`{}`,
	}

	for _, body := range invalidBodies {
		resp, err := ts.Client().Post(
			ts.URL+"/users",
			"application/json",
			bytes.NewReader([]byte(body)),
		)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// В БД не должно быть ни одной записи
	assert.Equal(t, 0, countUsers(t), "невалидные запросы не должны создавать записей в БД")
}
