package tests

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/http/pkg/handler"
)

var (
	ts         *httptest.Server
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// weatherBaseURL возвращает базовый URL для HTTP Weather тестов
func weatherBaseURL() string {
	return ts.URL
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

// TestMain запускает in-process HTTP сервер через httptest перед тестами
func TestMain(m *testing.M) {
	// DI инлайн: storage → handler → router
	storage := handler.NewWeatherStorage()
	weatherHandler := handler.NewWeatherHandler(storage)
	r, err := handler.SetupRouter(weatherHandler)
	if err != nil {
		panic(err)
	}
	ts = httptest.NewServer(r)

	code := m.Run()

	ts.Close()
	os.Exit(code)
}

// HTTP Weather Service Tests

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

// ErrorResponse представляет ошибку API
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func TestHTTP_PutWeather_Success(t *testing.T) {
	reqBody := UpdateWeatherRequest{Temperature: 25.5}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/Moscow", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, "Moscow", weather.City)
	assert.Equal(t, 25.5, weather.Temperature)
	assert.False(t, weather.UpdatedAt.IsZero())
}

func TestHTTP_PutWeather_UpdateExisting(t *testing.T) {
	city := "UpdateTest"

	reqBody1 := UpdateWeatherRequest{Temperature: 10.0}
	jsonBody1, err := json.Marshal(reqBody1)
	require.NoError(t, err)
	req1, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody1))
	require.NoError(t, err)
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := httpClient.Do(req1)
	require.NoError(t, err)
	resp1.Body.Close()

	reqBody2 := UpdateWeatherRequest{Temperature: 20.0}
	jsonBody2, err := json.Marshal(reqBody2)
	require.NoError(t, err)
	req2, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody2))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := httpClient.Do(req2)
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

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHTTP_GetWeather_Success(t *testing.T) {
	city := "GetTestCity"

	reqBody := UpdateWeatherRequest{Temperature: 15.0}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)
	putReq, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := httpClient.Do(putReq)
	require.NoError(t, err)
	putResp.Body.Close()

	getResp, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/" + city)
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
	resp, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/NonExistentCity")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHTTP_Weather_FullLifecycle(t *testing.T) {
	city := "LifecycleTestCity" + randomLetters(8)

	// 1. GET - не найден
	getResp1, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/" + city)
	require.NoError(t, err)
	getResp1.Body.Close()
	require.Equal(t, http.StatusNotFound, getResp1.StatusCode)

	// 2. PUT - создаём
	reqBody1 := UpdateWeatherRequest{Temperature: 22.5}
	jsonBody1, err := json.Marshal(reqBody1)
	require.NoError(t, err)
	putReq1, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody1))
	require.NoError(t, err)
	putReq1.Header.Set("Content-Type", "application/json")
	putResp1, err := httpClient.Do(putReq1)
	require.NoError(t, err)
	putResp1.Body.Close()
	require.Equal(t, http.StatusOK, putResp1.StatusCode)

	// 3. GET - теперь найден
	getResp2, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/" + city)
	require.NoError(t, err)
	defer getResp2.Body.Close()
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(getResp2.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, city, weather.City)
	assert.Equal(t, 22.5, weather.Temperature)

	// 4. PUT - обновляем
	reqBody2 := UpdateWeatherRequest{Temperature: 30.0}
	jsonBody2, err := json.Marshal(reqBody2)
	require.NoError(t, err)
	putReq2, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody2))
	require.NoError(t, err)
	putReq2.Header.Set("Content-Type", "application/json")
	putResp2, err := httpClient.Do(putReq2)
	require.NoError(t, err)
	putResp2.Body.Close()

	// 5. GET - проверяем обновлённые данные
	getResp3, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/" + city)
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

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)
	assert.Equal(t, -15.5, weather.Temperature)
}

func TestHTTP_PutWeather_ZeroTemperature(t *testing.T) {
	reqBody := UpdateWeatherRequest{Temperature: 0}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/ZeroCity", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, 0.0, weather.Temperature)
}

func TestHTTP_GetWeather_CityWithSpaces(t *testing.T) {
	city := "New York"
	encodedCity := "New%20York"

	reqBody := UpdateWeatherRequest{Temperature: 18.0}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)
	putReq, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+encodedCity, bytes.NewReader(jsonBody))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := httpClient.Do(putReq)
	require.NoError(t, err)
	putResp.Body.Close()

	getResp, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/" + encodedCity)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var weather WeatherResponse
	err = json.NewDecoder(getResp.Body).Decode(&weather)
	require.NoError(t, err)

	assert.Equal(t, city, weather.City)
	assert.Equal(t, 18.0, weather.Temperature)
}

func TestHTTP_GetWeather_VerifyDataPersists(t *testing.T) {
	city := "PersistCity"
	temperature := 12.5

	reqBody := UpdateWeatherRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)
	putReq, err := http.NewRequest(http.MethodPut, weatherBaseURL()+"/api/v1/weather/"+city, bytes.NewReader(jsonBody))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := httpClient.Do(putReq)
	require.NoError(t, err)
	putResp.Body.Close()

	for i := range 3 {
		getResp, err := httpClient.Get(weatherBaseURL() + "/api/v1/weather/" + city)
		require.NoError(t, err, "итерация %d", i)

		var weather WeatherResponse
		err = json.NewDecoder(getResp.Body).Decode(&weather)
		getResp.Body.Close()
		require.NoError(t, err, "итерация %d", i)

		assert.Equal(t, temperature, weather.Temperature, "итерация %d", i)
	}
}

func TestHTTP_PutWeather_EmptyBody(t *testing.T) {
	req, err := http.NewRequest(
		http.MethodPut,
		weatherBaseURL()+"/api/v1/weather/EmptyBodyCity",
		bytes.NewReader([]byte{}),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHTTP_PutWeather_LongCityName(t *testing.T) {
	longCity := strings.Repeat("LongCityXX", 100)
	temperature := 20.0

	reqBody := UpdateWeatherRequest{Temperature: temperature}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPut,
		weatherBaseURL()+"/api/v1/weather/"+longCity,
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest,
		"ожидается 200 или 400, получен %d", resp.StatusCode)
}
