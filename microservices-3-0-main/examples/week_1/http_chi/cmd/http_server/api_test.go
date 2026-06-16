package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_chi/pkg/models"
)

var ts *httptest.Server

// TestMain запускает in-memory HTTP сервер через httptest перед тестами
func TestMain(m *testing.M) {
	storage := models.NewWeatherStorage()
	ts = httptest.NewServer(setupRouter(storage))

	code := m.Run()

	ts.Close()
	os.Exit(code)
}

// WeatherResponse представляет ответ API погоды
type WeatherResponse struct {
	City        string    `json:"city"`
	Temperature float64   `json:"temperature"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WeatherUpdateRequest представляет запрос на обновление погоды
type WeatherUpdateRequest struct {
	Temperature float64 `json:"temperature"`
}

// assertHTTPStatus проверяет HTTP статус код ответа
func assertHTTPStatus(t *testing.T, resp *http.Response, expectedCode int) {
	t.Helper()
	assert.Equal(t, expectedCode, resp.StatusCode, "неверный HTTP статус код")
}

// ============ Update (PUT) Tests ============.

func TestPutWeather_Success(t *testing.T) {
	city := "TestCity"
	temperature := 25.5

	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assertHTTPStatus(t, resp, http.StatusOK)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, city, weather.City)
	assert.InDelta(t, temperature, weather.Temperature, 0.01)
	assert.False(t, weather.UpdatedAt.IsZero(), "updated_at должен быть установлен")
}

func TestPutWeather_UpdateExisting(t *testing.T) {
	city := "UpdateCity"

	// Первое обновление
	reqBody1 := WeatherUpdateRequest{Temperature: 10.0}
	jsonBody1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody1),
	)
	require.NoError(t, err)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := ts.Client().Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	var weather1 WeatherResponse
	err = json.NewDecoder(resp1.Body).Decode(&weather1)
	require.NoError(t, err)

	// Второе обновление - меняем температуру
	reqBody2 := WeatherUpdateRequest{Temperature: 30.0}
	jsonBody2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody2),
	)
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := ts.Client().Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var weather2 WeatherResponse
	err = json.NewDecoder(resp2.Body).Decode(&weather2)
	require.NoError(t, err)

	assert.Equal(t, city, weather2.City)
	assert.InDelta(t, 30.0, weather2.Temperature, 0.01)
	assert.NotEqual(t, weather1.Temperature, weather2.Temperature)
}

func TestPutWeather_InvalidBody(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/InvalidCity",
		bytes.NewReader([]byte("invalid json")),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assertHTTPStatus(t, resp, http.StatusBadRequest)
}

// ============ Get Tests ============.

func TestGetWeather_Success(t *testing.T) {
	city := "GetTestCity"
	temperature := 15.5

	// Сначала создаём данные через PUT
	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	putReq, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := ts.Client().Do(putReq)
	require.NoError(t, err)
	defer putResp.Body.Close()
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// Получаем данные через GET
	getResp, err := ts.Client().Get(ts.URL + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assertHTTPStatus(t, getResp, http.StatusOK)

	var weather WeatherResponse
	err = json.NewDecoder(getResp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, city, weather.City)
	assert.InDelta(t, temperature, weather.Temperature, 0.01)
	assert.False(t, weather.UpdatedAt.IsZero(), "updated_at должен быть установлен")
}

func TestGetWeather_NotFound(t *testing.T) {
	resp, err := ts.Client().Get(ts.URL + "/api/v1/weather/NonExistentCity")
	require.NoError(t, err)
	defer resp.Body.Close()

	assertHTTPStatus(t, resp, http.StatusNotFound)
}

// ============ Full Lifecycle Test ============.

func TestWeather_FullLifecycle(t *testing.T) {
	city := "LifecycleCity"

	// 1. GET несуществующего города - 404
	getResp1, err := ts.Client().Get(ts.URL + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp1.Body.Close()
	assert.Equal(t, http.StatusNotFound, getResp1.StatusCode, "несуществующий город должен вернуть 404")

	// 2. PUT - создаём данные о погоде
	reqBody1 := WeatherUpdateRequest{Temperature: 20.0}
	jsonBody1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	putReq1, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody1),
	)
	require.NoError(t, err)
	putReq1.Header.Set("Content-Type", "application/json")

	putResp1, err := ts.Client().Do(putReq1)
	require.NoError(t, err)
	defer putResp1.Body.Close()
	require.Equal(t, http.StatusOK, putResp1.StatusCode)

	var weather1 WeatherResponse
	err = json.NewDecoder(putResp1.Body).Decode(&weather1)
	require.NoError(t, err)
	assert.InDelta(t, 20.0, weather1.Temperature, 0.01)

	// 3. GET - проверяем данные
	getResp2, err := ts.Client().Get(ts.URL + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp2.Body.Close()
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var weather2 WeatherResponse
	err = json.NewDecoder(getResp2.Body).Decode(&weather2)
	require.NoError(t, err)
	assert.Equal(t, city, weather2.City)
	assert.InDelta(t, 20.0, weather2.Temperature, 0.01)

	// 4. PUT - обновляем температуру
	reqBody2 := WeatherUpdateRequest{Temperature: -5.0}
	jsonBody2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	putReq2, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody2),
	)
	require.NoError(t, err)
	putReq2.Header.Set("Content-Type", "application/json")

	putResp2, err := ts.Client().Do(putReq2)
	require.NoError(t, err)
	defer putResp2.Body.Close()
	require.Equal(t, http.StatusOK, putResp2.StatusCode)

	// 5. GET - проверяем обновлённые данные
	getResp3, err := ts.Client().Get(ts.URL + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp3.Body.Close()
	require.Equal(t, http.StatusOK, getResp3.StatusCode)

	var weather3 WeatherResponse
	err = json.NewDecoder(getResp3.Body).Decode(&weather3)
	require.NoError(t, err)
	assert.Equal(t, city, weather3.City)
	assert.InDelta(t, -5.0, weather3.Temperature, 0.01)
	assert.True(t, weather3.UpdatedAt.After(weather2.UpdatedAt) || weather3.UpdatedAt.Equal(weather2.UpdatedAt),
		"время обновления должно быть >= предыдущего")
}

// ============ Edge Cases ============.

func TestPutWeather_NegativeTemperature(t *testing.T) {
	city := "ColdCity"
	temperature := -40.0

	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assertHTTPStatus(t, resp, http.StatusOK)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.InDelta(t, temperature, weather.Temperature, 0.01)
}

func TestPutWeather_ZeroTemperature(t *testing.T) {
	city := "ZeroCity"
	temperature := 0.0

	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assertHTTPStatus(t, resp, http.StatusOK)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.InDelta(t, temperature, weather.Temperature, 0.01)
}

func TestGetWeather_CityWithSpaces(t *testing.T) {
	city := "New%20York" // URL-encoded space
	temperature := 22.0

	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Город создаётся
	assertHTTPStatus(t, resp, http.StatusOK)
}

func TestPutWeather_EmptyBody(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/EmptyBodyCity",
		bytes.NewReader([]byte{}),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assertHTTPStatus(t, resp, http.StatusBadRequest)
}

func TestPutWeather_LongCityName(t *testing.T) {
	// Очень длинное имя города (1000 символов)
	longCity := strings.Repeat("LongCity12", 100)
	temperature := 20.0

	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+longCity,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Сервер должен принять или вернуть ошибку, но не упасть
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest,
		"ожидается 200 или 400, получен %d", resp.StatusCode)
}

func TestPutWeather_SpecialChars(t *testing.T) {
	// Спецсимволы в имени города (URL-encoded)
	city := "City%2FWith%2FSlashes"
	temperature := 18.0

	reqBody := WeatherUpdateRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		ts.URL+"/api/v1/weather/"+city,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Проверяем, что сервер обрабатывает спецсимволы (может вернуть 200 или 404 в зависимости от роутера)
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound,
		"ожидается 200 или 404, получен %d", resp.StatusCode)
}
