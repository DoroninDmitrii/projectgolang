package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx как database/sql драйвер для goose
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	weatherAPI "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/api/weather/v1"
	weatherRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/repository/weather"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/openapi/weather/v1"
)

var (
	// httpAddr адрес in-process HTTP сервера (Weather Service)
	httpAddr string

	// httpClientInstance HTTP клиент для тестов Weather Service
	httpClientInstance = &http.Client{
		Timeout: 10 * time.Second,
	}
)

// weatherBaseURL возвращает базовый URL для HTTP Weather тестов
func weatherBaseURL() string {
	return "http://" + httpAddr
}

// randomLetters генерирует случайную строку из букв заданной длины
func randomLetters(length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, length)
	for i := range result {
		result[i] = letters[rand.IntN(len(letters))]
	}
	return string(result)
}

// WeatherResponse представляет ответ Weather API
type WeatherResponse struct {
	City        string    `json:"city"`
	Temperature float64   `json:"temperature"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpdateWeatherRequest представляет запрос на обновление погоды
type UpdateWeatherRequest struct {
	Temperature float64 `json:"temperature"`
}

// TestMain запускает тестовый PostgreSQL контейнер и HTTP сервер перед тестами
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Читаем креды из .env
	_ = godotenv.Load("../../http.env")

	// Поднимаем PostgreSQL через testcontainers
	container, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase(os.Getenv("POSTGRES_DB")),
		postgres.WithUsername(os.Getenv("POSTGRES_USER")),
		postgres.WithPassword(os.Getenv("POSTGRES_PASSWORD")),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	dbURI, err := container.ConnectionString(ctx, "sslmode=disable")
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

	if err = goose.Up(sqlDB, "../../migrations/http"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}

	sqlDB.Close()

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// DI инлайн: repository → api
	repo := weatherRepo.NewRepository(pool)
	weatherHandler := weatherAPI.NewAPI(repo)

	weatherServer, err := weatherv1.NewServer(weatherHandler)
	if err != nil {
		slog.Error("не удалось создать OpenAPI сервер", "error", err)
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Handle("/api/*", weatherServer)

	httpSrv := httptest.NewServer(r)
	// Убираем схему http:// — тесты используют httpAddr как "host:port".
	httpAddr = httpSrv.Listener.Addr().String()

	code := m.Run()

	httpSrv.Close()
	pool.Close()
	_ = container.Terminate(ctx)

	os.Exit(code)
}

// HTTP Weather Service Tests

func TestHTTP_PutWeather_Success(t *testing.T) {
	reqBody := UpdateWeatherRequest{Temperature: 25.5}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/Moscow", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClientInstance.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ожидается статус 200")

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, "Moscow", weather.City)
	assert.Equal(t, 25.5, weather.Temperature)
	assert.False(t, weather.UpdatedAt.IsZero())
}

func TestHTTP_PutWeather_UpdateExisting(t *testing.T) {
	city := "UpdateTest" + randomLetters(6)

	// Первое обновление
	reqBody1 := UpdateWeatherRequest{Temperature: 10.0}
	jsonBody1, err := json.Marshal(reqBody1)
	require.NoError(t, err)
	req1, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody1))
	require.NoError(t, err)
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := httpClientInstance.Do(req1)
	require.NoError(t, err)
	resp1.Body.Close()

	// Второе обновление
	reqBody2 := UpdateWeatherRequest{Temperature: 20.0}
	jsonBody2, err := json.Marshal(reqBody2)
	require.NoError(t, err)
	req2, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody2))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := httpClientInstance.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	var weather WeatherResponse
	err = json.NewDecoder(resp2.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, 20.0, weather.Temperature)
}

func TestHTTP_PutWeather_InvalidBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/TestCity", bytes.NewReader([]byte("invalid json")))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClientInstance.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHTTP_GetWeather_Success(t *testing.T) {
	city := "GetTest" + randomLetters(6)

	// Сначала создаём данные
	reqBody := UpdateWeatherRequest{Temperature: 15.0}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)
	putReq, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := httpClientInstance.Do(putReq)
	require.NoError(t, err)
	putResp.Body.Close()

	// Затем получаем
	getResp, err := httpClientInstance.Get(weatherBaseURL() + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(getResp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, city, weather.City)
	assert.Equal(t, 15.0, weather.Temperature)
}

func TestHTTP_GetWeather_NotFound(t *testing.T) {
	resp, err := httpClientInstance.Get(weatherBaseURL() + "/api/v1/weather/NonExistentCity")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHTTP_Weather_FullLifecycle(t *testing.T) {
	city := "Lifecycle" + randomLetters(8)

	// 1. GET — не найден
	getResp1, err := httpClientInstance.Get(weatherBaseURL() + "/api/v1/weather/" + city)
	require.NoError(t, err)
	getResp1.Body.Close()
	require.Equal(t, http.StatusNotFound, getResp1.StatusCode)

	// 2. PUT — создаём
	reqBody1 := UpdateWeatherRequest{Temperature: 22.5}
	jsonBody1, err := json.Marshal(reqBody1)
	require.NoError(t, err)
	putReq1, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody1))
	require.NoError(t, err)
	putReq1.Header.Set("Content-Type", "application/json")
	putResp1, err := httpClientInstance.Do(putReq1)
	require.NoError(t, err)
	putResp1.Body.Close()
	require.Equal(t, http.StatusOK, putResp1.StatusCode)

	// 3. GET — теперь найден
	getResp2, err := httpClientInstance.Get(weatherBaseURL() + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp2.Body.Close()
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(getResp2.Body).Decode(&weather)
	require.NoError(t, err)
	assert.Equal(t, city, weather.City)
	assert.Equal(t, 22.5, weather.Temperature)

	// 4. PUT — обновляем
	reqBody2 := UpdateWeatherRequest{Temperature: 30.0}
	jsonBody2, err := json.Marshal(reqBody2)
	require.NoError(t, err)
	putReq2, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody2))
	require.NoError(t, err)
	putReq2.Header.Set("Content-Type", "application/json")
	putResp2, err := httpClientInstance.Do(putReq2)
	require.NoError(t, err)
	putResp2.Body.Close()

	// 5. GET — проверяем обновлённые данные
	getResp3, err := httpClientInstance.Get(weatherBaseURL() + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp3.Body.Close()

	var updatedWeather WeatherResponse
	err = json.NewDecoder(getResp3.Body).Decode(&updatedWeather)
	require.NoError(t, err)
	assert.Equal(t, 30.0, updatedWeather.Temperature)
}

func TestHTTP_PutWeather_NegativeTemperature(t *testing.T) {
	reqBody := UpdateWeatherRequest{Temperature: -15.5}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/ColdCity", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClientInstance.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)
	assert.Equal(t, -15.5, weather.Temperature)
}

func TestHTTP_PutWeather_EmptyBody(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodPut,
		weatherBaseURL()+"/api/v1/weather/EmptyBodyCity",
		bytes.NewReader([]byte{}),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClientInstance.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
