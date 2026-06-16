package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/grpc_backend/pkg/interceptor"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/grpc_backend/pkg/service"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/http_frontend/pkg/handler"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/http_frontend/pkg/middleware"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/proto/ufo/v1"
)

const (
	bufSize    = 1024 * 1024
	validToken = "token-admin-123"
)

var (
	lis        *bufconn.Listener
	ts         *httptest.Server
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func baseURL() string {
	return ts.URL
}

// TestMain запускает gRPC backend (с auth interceptor) через bufconn и HTTP frontend через httptest
func TestMain(m *testing.M) {
	// gRPC backend с auth interceptor
	lis = bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.GRPCAuth),
	)
	ufov1.RegisterUFOServiceServer(grpcServer, service.NewEmpty())

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			panic(serveErr)
		}
	}()

	// gRPC клиент через bufconn
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}
	ufoClient := ufov1.NewUFOServiceClient(conn)

	// HTTP frontend с Chi + auth middleware
	h := handler.New(ufoClient)

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.HTTPAuth)
		r.Get("/sightings", h.ListSightings)
		r.Post("/sightings", h.CreateSighting)
		r.Get("/sightings/{id}", h.GetSighting)
	})

	ts = httptest.NewServer(r)

	code := m.Run()

	ts.Close()
	conn.Close()
	grpcServer.Stop()
	os.Exit(code)
}

// SightingResponse представляет наблюдение в HTTP ответе
type SightingResponse struct {
	ID          string `json:"id"`
	Location    string `json:"location"`
	Description string `json:"description"`
	ObservedAt  string `json:"observed_at"`
	CreatedBy   string `json:"created_by"`
}

// CreateSightingRequest представляет запрос на создание наблюдения
type CreateSightingRequest struct {
	Location    string `json:"location"`
	Description string `json:"description"`
}

// ErrorResponse представляет ошибку в HTTP ответе
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Тесты отклонения без авторизации

func TestNoToken_Returns401(t *testing.T) {
	resp, err := httpClient.Get(baseURL() + "/api/v1/sightings")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestInvalidToken_Returns401(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL()+"/api/v1/sightings", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid-token-xxx")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBadFormat_Returns401(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL()+"/api/v1/sightings", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Basic sometoken")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// Тесты CRUD с авторизацией

func authGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+validToken)

	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	return resp
}

func authPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+validToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	return resp
}

func TestList_Empty(t *testing.T) {
	resp := authGet(t, baseURL()+"/api/v1/sightings")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var sightings []SightingResponse
	err := json.NewDecoder(resp.Body).Decode(&sightings)
	require.NoError(t, err)
	// Может быть 0 или больше — NewEmpty() не имеет предзаполненных данных,
	// но другие тесты могли создать записи
	assert.NotNil(t, sightings)
}

func TestCreate_Success(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "Moscow, Russia",
		Description: "Тестовое наблюдение НЛО из теста авторизации",
	}

	resp := authPost(t, baseURL()+"/api/v1/sightings", reqBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var sighting SightingResponse
	err := json.NewDecoder(resp.Body).Decode(&sighting)
	require.NoError(t, err)

	assert.NotEmpty(t, sighting.ID)
	assert.Equal(t, reqBody.Location, sighting.Location)
	assert.Equal(t, reqBody.Description, sighting.Description)
	assert.NotEmpty(t, sighting.ObservedAt)
	// Токен admin → email admin@example.com
	assert.Equal(t, "admin@example.com", sighting.CreatedBy)
}

func TestCreate_TokenPropagation(t *testing.T) {
	// Создаём от имени admin
	reqBody := CreateSightingRequest{
		Location:    "Тест передачи токена",
		Description: "Проверка передачи токена HTTP → gRPC",
	}

	resp := authPost(t, baseURL()+"/api/v1/sightings", reqBody)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created SightingResponse
	err := json.NewDecoder(resp.Body).Decode(&created)
	require.NoError(t, err)

	// created_by должен быть заполнен из контекста auth
	assert.Equal(t, "admin@example.com", created.CreatedBy,
		"token propagation: HTTP Bearer → gRPC metadata → service context → created_by")

	// Получаем обратно и проверяем, что created_by сохранён
	getResp := authGet(t, baseURL()+"/api/v1/sightings/"+created.ID)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var fetched SightingResponse
	err = json.NewDecoder(getResp.Body).Decode(&fetched)
	require.NoError(t, err)

	assert.Equal(t, "admin@example.com", fetched.CreatedBy)
}

func TestCreate_EmptyLocation(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "",
		Description: "Тестовое описание",
	}

	resp := authPost(t, baseURL()+"/api/v1/sightings", reqBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreate_EmptyDescription(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "Тестовая локация",
		Description: "",
	}

	resp := authPost(t, baseURL()+"/api/v1/sightings", reqBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreate_InvalidJSON(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL()+"/api/v1/sightings", bytes.NewReader([]byte("invalid json")))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+validToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGet_NotFound(t *testing.T) {
	resp := authGet(t, baseURL()+"/api/v1/sightings/non-existent-id")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "not_found", errResp.Error)
}

func TestFullLifecycle(t *testing.T) {
	// 1. Получаем список — запоминаем количество
	listResp1 := authGet(t, baseURL()+"/api/v1/sightings")
	defer listResp1.Body.Close()

	var sightings1 []SightingResponse
	err := json.NewDecoder(listResp1.Body).Decode(&sightings1)
	require.NoError(t, err)
	initialCount := len(sightings1)

	// 2. Создаём новое наблюдение
	reqBody := CreateSightingRequest{
		Location:    "Тестовая локация жизненного цикла",
		Description: "Тестовое описание жизненного цикла",
	}

	createResp := authPost(t, baseURL()+"/api/v1/sightings", reqBody)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created SightingResponse
	err = json.NewDecoder(createResp.Body).Decode(&created)
	require.NoError(t, err)

	// 3. Получаем созданное наблюдение
	getResp := authGet(t, baseURL()+"/api/v1/sightings/"+created.ID)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var fetched SightingResponse
	err = json.NewDecoder(getResp.Body).Decode(&fetched)
	require.NoError(t, err)

	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, reqBody.Location, fetched.Location)
	assert.Equal(t, reqBody.Description, fetched.Description)
	assert.Equal(t, "admin@example.com", fetched.CreatedBy)

	// 4. Список увеличился на 1.
	listResp2 := authGet(t, baseURL()+"/api/v1/sightings")
	defer listResp2.Body.Close()

	var sightings2 []SightingResponse
	err = json.NewDecoder(listResp2.Body).Decode(&sightings2)
	require.NoError(t, err)

	assert.Equal(t, initialCount+1, len(sightings2))

	// 5. Новое наблюдение есть в списке
	found := false
	for _, s := range sightings2 {
		if s.ID == created.ID {
			found = true
			assert.Equal(t, reqBody.Location, s.Location)
			break
		}
	}
	assert.True(t, found, "созданное наблюдение должно быть в списке")
}

func TestDifferentUsers(t *testing.T) {
	tokens := map[string]string{
		"token-admin-123": "admin@example.com",
		"token-user-456":  "user@example.com",
	}

	for token, expectedEmail := range tokens {
		reqBody := CreateSightingRequest{
			Location:    "Тест пользователя " + expectedEmail,
			Description: "Проверка created_by для " + expectedEmail,
		}
		jsonBody, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL()+"/api/v1/sightings", bytes.NewReader(jsonBody))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var sighting SightingResponse
		err = json.NewDecoder(resp.Body).Decode(&sighting)
		require.NoError(t, err)

		assert.Equal(t, expectedEmail, sighting.CreatedBy,
			"created_by должен соответствовать токену пользователя")
	}
}
