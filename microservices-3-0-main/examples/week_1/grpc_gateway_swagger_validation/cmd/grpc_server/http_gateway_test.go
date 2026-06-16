package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_gateway_swagger_validation/pkg/proto/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_gateway_swagger_validation/static"
)

var (
	httpServer     *httptest.Server
	httpServerOnce sync.Once
)

const (
	swaggerUITestFile   = "swagger-ui.html"
	swaggerJSONTestFile = "generated/ufo.swagger.json"
)

// getHTTPServer возвращает httptest сервер с gRPC-Gateway, подключённый к bufconn gRPC серверу
func getHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()

	httpServerOnce.Do(func() {
		// Создаём gRPC клиент через bufconn
		conn, err := grpc.NewClient(
			"passthrough:///bufnet",
			grpc.WithContextDialer(bufDialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			panic(err)
		}

		// Создаём gRPC-Gateway mux
		mux := runtime.NewServeMux()
		err = ufov1.RegisterUFOServiceHandler(context.Background(), mux, conn)
		if err != nil {
			panic(err)
		}

		// Создаём HTTP mux с /api/, swagger-ui.html, swagger.json
		httpMux := http.NewServeMux()
		httpMux.Handle("/api/", mux)

		httpMux.Handle("/swagger-ui.html", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			data, readErr := static.FS.ReadFile(swaggerUITestFile)
			if readErr != nil {
				http.Error(w, "swagger-ui.html not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
		}))
		httpMux.Handle("/swagger.json", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			data, readErr := static.FS.ReadFile(swaggerJSONTestFile)
			if readErr != nil {
				http.Error(w, "swagger.json not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		}))

		httpServer = httptest.NewServer(httpMux)
	})

	return httpServer
}

// ============ HTTP Create Tests ============.

func TestHTTP_Create_Success(t *testing.T) {
	ts := getHTTPServer(t)

	body := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "HTTP Test Location",
			"description": "HTTP Test Description",
		},
	}

	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.NotEmpty(t, result["uuid"], "UUID должен быть в ответе")
}

func TestHTTP_Create_ValidationError(t *testing.T) {
	ts := getHTTPServer(t)

	body := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "",
			"description": "Test Description",
		},
	}

	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ============ HTTP Get Tests ============.

func TestHTTP_Get_Success(t *testing.T) {
	ts := getHTTPServer(t)

	// Создаём запись
	createBody := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "HTTP Get Location",
			"description": "HTTP Get Description",
		},
	}

	jsonBody, err := json.Marshal(createBody)
	require.NoError(t, err)

	createResp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	var createResult map[string]any
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)

	createdUUID, ok := createResult["uuid"].(string)
	require.True(t, ok, "uuid должен быть строкой")

	// Получаем запись
	getResp, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var getResult map[string]any
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)

	sighting, ok := getResult["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	assert.Equal(t, createdUUID, sighting["uuid"])
}

func TestHTTP_Get_NotFound(t *testing.T) {
	ts := getHTTPServer(t)

	nonExistentUUID := uuid.NewString()

	resp, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + nonExistentUUID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ============ HTTP Update Tests ============.

func TestHTTP_Update_Success(t *testing.T) {
	ts := getHTTPServer(t)

	// Создаём запись
	createBody := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "Original HTTP Location",
			"description": "Original HTTP Description",
		},
	}

	jsonBody, err := json.Marshal(createBody)
	require.NoError(t, err)

	createResp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]any
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)

	createdUUID, ok := createResult["uuid"].(string)
	require.True(t, ok, "uuid должен быть строкой")

	// Обновляем запись
	updateBody := map[string]any{
		"updateInfo": map[string]any{
			"location": "Updated HTTP Location",
		},
	}

	updateJSON, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/ufo/"+createdUUID,
		bytes.NewReader(updateJSON),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	updateResp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Проверяем обновление
	getResp, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp.Body.Close()

	var getResult map[string]any
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)

	sighting, ok := getResult["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	info, ok := sighting["info"].(map[string]any)
	require.True(t, ok, "info должен быть объектом")
	assert.Equal(t, "Updated HTTP Location", info["location"])
}

func TestHTTP_Update_NotFound(t *testing.T) {
	ts := getHTTPServer(t)

	nonExistentUUID := uuid.NewString()

	updateBody := map[string]any{
		"updateInfo": map[string]any{
			"location": "Updated Location",
		},
	}

	updateJSON, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/ufo/"+nonExistentUUID,
		bytes.NewReader(updateJSON),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ============ HTTP Delete Tests ============.

func TestHTTP_Delete_Success(t *testing.T) {
	ts := getHTTPServer(t)

	// Создаём запись
	createBody := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "HTTP Delete Location",
			"description": "HTTP Delete Description",
		},
	}

	jsonBody, err := json.Marshal(createBody)
	require.NoError(t, err)

	createResp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]any
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)

	createdUUID, ok := createResult["uuid"].(string)
	require.True(t, ok, "uuid должен быть строкой")

	// Удаляем запись
	req, err := http.NewRequest(
		http.MethodDelete,
		ts.URL+"/api/v1/ufo/"+createdUUID,
		nil,
	)
	require.NoError(t, err)

	deleteResp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusOK, deleteResp.StatusCode)

	// Проверяем, что запись помечена как удалённая
	getResp, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp.Body.Close()

	var getResult map[string]any
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)

	sighting, ok := getResult["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	assert.NotNil(t, sighting["deletedAt"], "deleted_at должен быть установлен")
}

func TestHTTP_Delete_NotFound(t *testing.T) {
	ts := getHTTPServer(t)

	nonExistentUUID := uuid.NewString()

	req, err := http.NewRequest(
		http.MethodDelete,
		ts.URL+"/api/v1/ufo/"+nonExistentUUID,
		nil,
	)
	require.NoError(t, err)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ============ Swagger UI Test ============.

func TestHTTP_SwaggerUI_Available(t *testing.T) {
	ts := getHTTPServer(t)

	resp, err := ts.Client().Get(ts.URL + "/swagger-ui.html")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Contains(t, string(body), "swagger", "должен содержать Swagger UI")
}

func TestHTTP_SwaggerJSON_Available(t *testing.T) {
	ts := getHTTPServer(t)

	resp, err := ts.Client().Get(ts.URL + "/swagger.json")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.NotEmpty(t, result, "swagger.json должен быть валидным JSON")
}

// ============ Full Lifecycle Test ============.

func TestHTTP_FullLifecycle(t *testing.T) {
	ts := getHTTPServer(t)

	// 1. Create
	createBody := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "HTTP Lifecycle Location",
			"description": "HTTP Lifecycle Description",
		},
	}

	jsonBody, err := json.Marshal(createBody)
	require.NoError(t, err)

	createResp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	var createResult map[string]any
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)

	createdUUID, ok := createResult["uuid"].(string)
	require.True(t, ok, "uuid должен быть строкой")
	assert.NotEmpty(t, createdUUID)

	// 2. Get - проверяем созданные данные
	getResp1, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp1.Body.Close()
	require.Equal(t, http.StatusOK, getResp1.StatusCode)

	var getResult1 map[string]any
	err = json.NewDecoder(getResp1.Body).Decode(&getResult1)
	require.NoError(t, err)

	sighting1, ok := getResult1["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	info1, ok := sighting1["info"].(map[string]any)
	require.True(t, ok, "info должен быть объектом")
	assert.Equal(t, "HTTP Lifecycle Location", info1["location"])
	assert.Nil(t, sighting1["deletedAt"])

	// 3. Update
	updateBody := map[string]any{
		"updateInfo": map[string]any{
			"location": "Updated HTTP Lifecycle Location",
		},
	}

	updateJSON, err := json.Marshal(updateBody)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/ufo/"+createdUUID,
		bytes.NewReader(updateJSON),
	)
	require.NoError(t, err)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := ts.Client().Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	// 4. Get - проверяем обновление
	getResp2, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp2.Body.Close()

	var getResult2 map[string]any
	err = json.NewDecoder(getResp2.Body).Decode(&getResult2)
	require.NoError(t, err)

	sighting2, ok := getResult2["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	info2, ok := sighting2["info"].(map[string]any)
	require.True(t, ok, "info должен быть объектом")
	assert.Equal(t, "Updated HTTP Lifecycle Location", info2["location"])
	assert.NotNil(t, sighting2["updatedAt"])

	// 5. Delete
	deleteReq, err := http.NewRequest(
		http.MethodDelete,
		ts.URL+"/api/v1/ufo/"+createdUUID,
		nil,
	)
	require.NoError(t, err)

	deleteResp, err := ts.Client().Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)

	// 6. Get - проверяем deleted_at
	getResp3, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp3.Body.Close()

	var getResult3 map[string]any
	err = json.NewDecoder(getResp3.Body).Decode(&getResult3)
	require.NoError(t, err)

	sighting3, ok := getResult3["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	assert.NotNil(t, sighting3["deletedAt"])
}

func TestHTTP_Update_ValidationError(t *testing.T) {
	ts := getHTTPServer(t)

	// Создаём запись
	createBody := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "Valid Location",
			"description": "Valid Description",
		},
	}

	jsonBody, err := json.Marshal(createBody)
	require.NoError(t, err)

	createResp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	var createResult map[string]any
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)

	createdUUID, ok := createResult["uuid"].(string)
	require.True(t, ok, "uuid должен быть строкой")

	updateBody := map[string]any{
		"updateInfo": map[string]any{
			"location": "",
		},
	}

	updateJSON, err := json.Marshal(updateBody)
	require.NoError(t, err)

	updateReq, err := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/ufo/"+createdUUID,
		bytes.NewReader(updateJSON),
	)
	require.NoError(t, err)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := ts.Client().Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)
}

func TestHTTP_Delete_AlreadyDeleted(t *testing.T) {
	ts := getHTTPServer(t)

	// Создаём запись
	createBody := map[string]any{
		"info": map[string]any{
			"observedAt":  time.Now().Format(time.RFC3339Nano),
			"location":    "Double Delete Location",
			"description": "Double Delete Description",
		},
	}

	jsonBody, err := json.Marshal(createBody)
	require.NoError(t, err)

	createResp, err := ts.Client().Post(
		ts.URL+"/api/v1/ufo",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]any
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)

	createdUUID, ok := createResult["uuid"].(string)
	require.True(t, ok, "uuid должен быть строкой")

	// Первое удаление
	deleteReq1, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/ufo/"+createdUUID, nil)
	require.NoError(t, err)
	deleteResp1, err := ts.Client().Do(deleteReq1)
	require.NoError(t, err)
	defer deleteResp1.Body.Close()
	require.Equal(t, http.StatusOK, deleteResp1.StatusCode)

	// Проверяем deleted_at
	getResp1, err := ts.Client().Get(ts.URL + "/api/v1/ufo/" + createdUUID)
	require.NoError(t, err)
	defer getResp1.Body.Close()

	var getResult1 map[string]any
	err = json.NewDecoder(getResp1.Body).Decode(&getResult1)
	require.NoError(t, err)

	sighting1, ok := getResult1["sighting"].(map[string]any)
	require.True(t, ok, "sighting должен быть объектом")
	assert.NotNil(t, sighting1["deletedAt"])

	// Второе удаление - идемпотентно
	deleteReq2, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/ufo/"+createdUUID, nil)
	require.NoError(t, err)
	deleteResp2, err := ts.Client().Do(deleteReq2)
	require.NoError(t, err)
	defer deleteResp2.Body.Close()

	require.Equal(t, http.StatusOK, deleteResp2.StatusCode)
}
