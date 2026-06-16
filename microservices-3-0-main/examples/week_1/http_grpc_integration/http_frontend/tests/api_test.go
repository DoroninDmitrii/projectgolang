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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/grpc_backend/pkg/service"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/http_frontend/pkg/handler"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/shared/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

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

// TestMain запускает gRPC backend через bufconn и HTTP frontend через httptest
func TestMain(m *testing.M) {
	// DI инлайн: gRPC backend
	lis = bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(grpcServer, service.New())

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

	// DI инлайн: HTTP frontend
	h := handler.New(ufoClient)
	ts = httptest.NewServer(handler.SetupMux(h))

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

// List Tests

func TestList_Success(t *testing.T) {
	resp, err := httpClient.Get(baseURL() + "/api/v1/sightings")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var sightings []SightingResponse
	err = json.NewDecoder(resp.Body).Decode(&sightings)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(sightings), 3, "должно быть минимум 3 предзаполненных наблюдения")
}

func TestList_ContainsPrePopulatedData(t *testing.T) {
	resp, err := httpClient.Get(baseURL() + "/api/v1/sightings")
	require.NoError(t, err)
	defer resp.Body.Close()

	var sightings []SightingResponse
	err = json.NewDecoder(resp.Body).Decode(&sightings)
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, s := range sightings {
		ids[s.ID] = true
	}

	assert.True(t, ids["sighting-001"], "должен быть sighting-001")
	assert.True(t, ids["sighting-002"], "должен быть sighting-002")
	assert.True(t, ids["sighting-003"], "должен быть sighting-003")
}

// Get Tests

func TestGet_Success(t *testing.T) {
	resp, err := httpClient.Get(baseURL() + "/api/v1/sightings/sighting-001")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var sighting SightingResponse
	err = json.NewDecoder(resp.Body).Decode(&sighting)
	require.NoError(t, err)

	assert.Equal(t, "sighting-001", sighting.ID)
	assert.Equal(t, "Roswell, New Mexico", sighting.Location)
	assert.NotEmpty(t, sighting.Description)
	assert.NotEmpty(t, sighting.ObservedAt)
}

func TestGet_NotFound(t *testing.T) {
	resp, err := httpClient.Get(baseURL() + "/api/v1/sightings/non-existent-id")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp ErrorResponse
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "not_found", errResp.Error)
}

func TestGet_EmptyID(t *testing.T) {
	resp, err := httpClient.Get(baseURL() + "/api/v1/sightings/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, resp.StatusCode)
}

// Create Tests

func TestCreate_Success(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "Moscow, Russia",
		Description: "Test UFO sighting from API test",
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := httpClient.Post(
		baseURL()+"/api/v1/sightings",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var sighting SightingResponse
	err = json.NewDecoder(resp.Body).Decode(&sighting)
	require.NoError(t, err)

	assert.NotEmpty(t, sighting.ID)
	assert.Equal(t, reqBody.Location, sighting.Location)
	assert.Equal(t, reqBody.Description, sighting.Description)
	assert.NotEmpty(t, sighting.ObservedAt)

	getResp, err := httpClient.Get(baseURL() + "/api/v1/sightings/" + sighting.ID)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var fetchedSighting SightingResponse
	err = json.NewDecoder(getResp.Body).Decode(&fetchedSighting)
	require.NoError(t, err)

	assert.Equal(t, sighting.ID, fetchedSighting.ID)
	assert.Equal(t, sighting.Location, fetchedSighting.Location)
}

func TestCreate_EmptyLocation(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "",
		Description: "Test description",
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := httpClient.Post(
		baseURL()+"/api/v1/sightings",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreate_EmptyDescription(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "Test Location",
		Description: "",
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := httpClient.Post(
		baseURL()+"/api/v1/sightings",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreate_InvalidJSON(t *testing.T) {
	resp, err := httpClient.Post(
		baseURL()+"/api/v1/sightings",
		"application/json",
		bytes.NewReader([]byte("invalid json")),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Full Lifecycle Test

func TestFullLifecycle(t *testing.T) {
	// 1. Получаем список - проверяем предзаполненные данные
	listResp1, err := httpClient.Get(baseURL() + "/api/v1/sightings")
	require.NoError(t, err)
	defer listResp1.Body.Close()

	var sightings1 []SightingResponse
	err = json.NewDecoder(listResp1.Body).Decode(&sightings1)
	require.NoError(t, err)
	initialCount := len(sightings1)

	// 2. Создаём новое наблюдение
	reqBody := CreateSightingRequest{
		Location:    "Lifecycle Test Location",
		Description: "Lifecycle Test Description",
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createResp, err := httpClient.Post(
		baseURL()+"/api/v1/sightings",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createdSighting SightingResponse
	err = json.NewDecoder(createResp.Body).Decode(&createdSighting)
	require.NoError(t, err)

	// 3. Получаем созданное наблюдение
	getResp, err := httpClient.Get(baseURL() + "/api/v1/sightings/" + createdSighting.ID)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var fetchedSighting SightingResponse
	err = json.NewDecoder(getResp.Body).Decode(&fetchedSighting)
	require.NoError(t, err)

	assert.Equal(t, createdSighting.ID, fetchedSighting.ID)
	assert.Equal(t, reqBody.Location, fetchedSighting.Location)
	assert.Equal(t, reqBody.Description, fetchedSighting.Description)

	// 4. Получаем обновлённый список
	listResp2, err := httpClient.Get(baseURL() + "/api/v1/sightings")
	require.NoError(t, err)
	defer listResp2.Body.Close()

	var sightings2 []SightingResponse
	err = json.NewDecoder(listResp2.Body).Decode(&sightings2)
	require.NoError(t, err)

	assert.Equal(t, initialCount+1, len(sightings2), "количество наблюдений должно увеличиться на 1")

	// 5. Проверяем, что новое наблюдение есть в списке
	found := false
	for _, s := range sightings2 {
		if s.ID == createdSighting.ID {
			found = true
			assert.Equal(t, reqBody.Location, s.Location)
			break
		}
	}
	assert.True(t, found, "созданное наблюдение должно быть в списке")
}

// Integration Tests

func TestHTTPToGRPCIntegration_PrePopulatedData(t *testing.T) {
	testCases := []struct {
		id          string
		location    string
		description string
	}{
		{"sighting-001", "Roswell, New Mexico", "Яркий диск в небе над пустыней"},
		{"sighting-002", "Phoenix, Arizona", "Треугольный объект с огнями"},
		{"sighting-003", "Москва, Россия", "Светящийся шар над ВДНХ"},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			resp, err := httpClient.Get(baseURL() + "/api/v1/sightings/" + tc.id)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var sighting SightingResponse
			err = json.NewDecoder(resp.Body).Decode(&sighting)
			require.NoError(t, err)

			assert.Equal(t, tc.id, sighting.ID)
			assert.Equal(t, tc.location, sighting.Location)
			assert.Contains(t, sighting.Description, tc.description[:10])
		})
	}
}

func TestHTTPToGRPCIntegration_CreateAndGet(t *testing.T) {
	reqBody := CreateSightingRequest{
		Location:    "Integration Test City",
		Description: "Integration Test Description",
	}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createResp, err := httpClient.Post(
		baseURL()+"/api/v1/sightings",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created SightingResponse
	err = json.NewDecoder(createResp.Body).Decode(&created)
	require.NoError(t, err)

	getResp, err := httpClient.Get(baseURL() + "/api/v1/sightings/" + created.ID)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var fetched SightingResponse
	err = json.NewDecoder(getResp.Body).Decode(&fetched)
	require.NoError(t, err)

	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Location, fetched.Location)
	assert.Equal(t, created.Description, fetched.Description)
	assert.Equal(t, created.ObservedAt, fetched.ObservedAt)
}
