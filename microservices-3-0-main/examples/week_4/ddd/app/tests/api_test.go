package tests

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/go-chi/chi/v5"
	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	pcBuilderAPI "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/api/pc_builder"
	buildRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/build"
	componentRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/component"
	pcBuilder "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/service/application/pc_builder"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/service/domain"
)

const (
	// UUID комплектующих из seed-данных
	motherboardZ790 = "cccccccc-0000-0000-0000-000000000001"
	motherboardB650 = "cccccccc-0000-0000-0000-000000000002"
	cpuIntelI7      = "cccccccc-0000-0000-0000-000000000011"
	cpuAMDRyzen9    = "cccccccc-0000-0000-0000-000000000012"
	ramDDR5         = "cccccccc-0000-0000-0000-000000000021"
	ramDDR4         = "cccccccc-0000-0000-0000-000000000022"
	gpuRTX4070      = "cccccccc-0000-0000-0000-000000000031"
	gpuRTX4090      = "cccccccc-0000-0000-0000-000000000032"
)

var (
	pool        *pgxpool.Pool
	router      *chi.Mux
	resetDataMu sync.Mutex
)

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции
// и собирает DI для HTTP-сервера
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Читаем креды из корневого app.env рядом с go.work
	_ = godotenv.Load("../../app.env")

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

	// Накатываем миграции через goose (включая seed-данные)
	sqlDB, err := sql.Open("pgx", dbURI)
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

	sqlDB.Close()

	// Подключаемся через pgxpool
	pool, err = pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// DI инлайн: transaction manager → репозитории → чекер → сервис → handler → router
	txManager, err := manager.New(trmpgx.NewDefaultFactory(pool))
	if err != nil {
		slog.Error("не удалось создать transaction manager", "error", err)
		os.Exit(1)
	}

	compRepo := componentRepo.NewRepository(pool)
	bldRepo := buildRepo.NewRepository(pool)
	checker := domain.NewCompatibilityChecker()
	svc := pcBuilder.NewService(txManager, compRepo, bldRepo, checker)
	handler := pcBuilderAPI.NewHandler(svc)

	// Собираем chi-роутер — точно как в app.go
	router = chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		r.Post("/builds", handler.CreateBuild)
		r.Post("/builds/{uuid}/cancel", handler.CancelBuild)
	})

	code := m.Run()

	pool.Close()
	_ = container.Terminate(ctx)

	os.Exit(code)
}

// resetData очищает таблицы и вставляет seed-данные заново
// Захватывает мьютекс на всё время выполнения теста, чтобы параллельные
// подтесты не мешали друг другу при работе с общей БД
func resetData(t *testing.T) {
	t.Helper()

	resetDataMu.Lock()
	t.Cleanup(resetDataMu.Unlock)

	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM pc_build_components")
	require.NoError(t, err, "не удалось очистить таблицу pc_build_components")

	_, err = pool.Exec(ctx, "DELETE FROM pc_builds")
	require.NoError(t, err, "не удалось очистить таблицу pc_builds")

	_, err = pool.Exec(ctx, "DELETE FROM components")
	require.NoError(t, err, "не удалось очистить таблицу components")

	_, err = pool.Exec(ctx, `
		INSERT INTO components (uuid, name, type, properties, stock_quantity) VALUES
		('cccccccc-0000-0000-0000-000000000001', 'ASUS ROG Strix Z790', 'motherboard',
		 '{"motherboard": {"socket": "LGA1700", "ram_type": "DDR5", "ram_slots": 4}}', 3),
		('cccccccc-0000-0000-0000-000000000002', 'MSI MAG B650', 'motherboard',
		 '{"motherboard": {"socket": "AM5", "ram_type": "DDR5", "ram_slots": 2}}', 2),
		('cccccccc-0000-0000-0000-000000000011', 'Intel Core i7-13700K', 'cpu',
		 '{"cpu": {"socket": "LGA1700", "cores": 16, "tdp_watts": 125}}', 5),
		('cccccccc-0000-0000-0000-000000000012', 'AMD Ryzen 9 7950X', 'cpu',
		 '{"cpu": {"socket": "AM5", "cores": 16, "tdp_watts": 170}}', 3),
		('cccccccc-0000-0000-0000-000000000021', 'Kingston Fury Beast DDR5', 'ram',
		 '{"ram": {"ram_type": "DDR5", "capacity_gb": 32}}', 10),
		('cccccccc-0000-0000-0000-000000000022', 'Corsair Vengeance DDR4', 'ram',
		 '{"ram": {"ram_type": "DDR4", "capacity_gb": 16}}', 8),
		('cccccccc-0000-0000-0000-000000000031', 'NVIDIA RTX 4070', 'gpu',
		 '{"gpu": {"required_tdp_watts": 200, "vram_gb": 12}}', 4),
		('cccccccc-0000-0000-0000-000000000032', 'NVIDIA RTX 4090', 'gpu',
		 '{"gpu": {"required_tdp_watts": 450, "vram_gb": 24}}', 1)
	`)
	require.NoError(t, err, "не удалось вставить seed-данные")
}

// createBuildRequest выполняет POST /api/v1/builds и возвращает recorder
func createBuildRequest(t *testing.T, componentUUIDs []string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"component_uuids": componentUUIDs,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/builds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w
}

// cancelBuildRequest выполняет POST /api/v1/builds/{uuid}/cancel и возвращает recorder
func cancelBuildRequest(t *testing.T, buildUUID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/builds/%s/cancel", buildUUID), nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w
}

// createBuildResponse — структура JSON-ответа создания сборки
type createBuildResponse struct {
	BuildUUID string `json:"build_uuid"`
	Status    string `json:"status"`
}

// cancelBuildResponse — структура JSON-ответа отмены сборки
type cancelBuildResponse struct {
	BuildUUID string `json:"build_uuid"`
	Status    string `json:"status"`
}

// errorResponse — структура JSON-ответа с ошибкой
type errorResponse struct {
	Error string `json:"error"`
}

// getReserved возвращает количество зарезервированных единиц компонента
func getReserved(t *testing.T, uuid string) int {
	t.Helper()

	var reserved int
	err := pool.QueryRow(context.Background(),
		"SELECT reserved FROM components WHERE uuid = $1", uuid).Scan(&reserved)
	require.NoError(t, err, "не удалось получить reserved")

	return reserved
}

// getAvailable возвращает количество доступных единиц компонента
func getAvailable(t *testing.T, uuid string) int {
	t.Helper()

	var available int
	err := pool.QueryRow(context.Background(),
		"SELECT stock_quantity - reserved FROM components WHERE uuid = $1", uuid).Scan(&available)
	require.NoError(t, err, "не удалось получить available")

	return available
}

// getBuildStatus возвращает статус сборки
func getBuildStatus(t *testing.T, uuid string) string {
	t.Helper()

	var status string
	err := pool.QueryRow(context.Background(),
		"SELECT status FROM pc_builds WHERE uuid = $1", uuid).Scan(&status)
	require.NoError(t, err, "не удалось получить статус сборки")

	return status
}

// TestCreateBuild проверяет создание сборок с разными комбинациями комплектующих
func TestCreateBuild(t *testing.T) {
	tests := []struct {
		name           string
		componentUUIDs []string
		wantHTTPStatus int
		wantStatus     string
		wantErr        string
	}{
		{
			name:           "совместимая Intel-сборка",
			componentUUIDs: []string{motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4070},
			wantHTTPStatus: http.StatusCreated,
			wantStatus:     "reserved",
		},
		{
			name:           "совместимая AMD-сборка",
			componentUUIDs: []string{motherboardB650, cpuAMDRyzen9, ramDDR5, gpuRTX4070},
			wantHTTPStatus: http.StatusCreated,
			wantStatus:     "reserved",
		},
		{
			name:           "несовместимый сокет (B650 + Intel i7)",
			componentUUIDs: []string{motherboardB650, cpuIntelI7, ramDDR5, gpuRTX4070},
			wantHTTPStatus: http.StatusUnprocessableEntity,
			wantErr:        "сокет процессора несовместим с материнской платой",
		},
		{
			name:           "несовместимая RAM (Z790 + DDR4)",
			componentUUIDs: []string{motherboardZ790, cpuIntelI7, ramDDR4, gpuRTX4070},
			wantHTTPStatus: http.StatusUnprocessableEntity,
			wantErr:        "тип оперативной памяти несовместим с материнской платой",
		},
		{
			name:           "несовместимый TDP (i7 + RTX 4090)",
			componentUUIDs: []string{motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4090},
			wantHTTPStatus: http.StatusUnprocessableEntity,
			wantErr:        "видеокарта требует больше мощности, чем процессор может обеспечить",
		},
		{
			name:           "без материнской платы",
			componentUUIDs: []string{cpuIntelI7, ramDDR5, gpuRTX4070},
			wantHTTPStatus: http.StatusUnprocessableEntity,
			wantErr:        "сборка должна содержать материнскую плату",
		},
		{
			name:           "несуществующий UUID компонента",
			componentUUIDs: []string{motherboardZ790, cpuIntelI7, ramDDR5, "00000000-0000-0000-0000-000000000099"},
			wantHTTPStatus: http.StatusNotFound,
			wantErr:        "компонент не найден",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resetData(t)

			w := createBuildRequest(t, tt.componentUUIDs)

			assert.Equal(t, tt.wantHTTPStatus, w.Code, "неверный HTTP-статус")
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Content-Type должен быть application/json")

			if tt.wantHTTPStatus == http.StatusCreated {
				var resp createBuildResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

				assert.NotEmpty(t, resp.BuildUUID, "UUID сборки не должен быть пустым")
				assert.Equal(t, tt.wantStatus, resp.Status, "статус должен быть reserved")

				// Проверяем состояние в БД
				status := getBuildStatus(t, resp.BuildUUID)
				assert.Equal(t, "reserved", status, "статус сборки в БД должен быть reserved")
			} else {
				var resp errorResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

				assert.Contains(t, resp.Error, tt.wantErr, "текст ошибки должен содержать ожидаемую строку")

				// При ошибке транзакция откатывается — reserved остаётся 0.
				for _, uuid := range tt.componentUUIDs {
					// Пропускаем несуществующие UUID
					if uuid == "00000000-0000-0000-0000-000000000099" {
						continue
					}
					assert.Equal(t, 0, getReserved(t, uuid),
						"reserved должен быть 0 после отката транзакции для %s", uuid)
				}
			}
		})
	}
}

// TestCreateBuild_InvalidJSON проверяет ответ на некорректный JSON
func TestCreateBuild_InvalidJSON(t *testing.T) {
	resetData(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/builds", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "некорректный JSON должен вернуть 400")
}

// TestCreateBuild_EmptyComponentUUIDs проверяет создание сборки с пустым списком компонентов
func TestCreateBuild_EmptyComponentUUIDs(t *testing.T) {
	resetData(t)

	w := createBuildRequest(t, []string{})

	// Пустой список → нет материнской платы → 422.
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "пустой список компонентов должен вернуть 422")

	var resp errorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "сборка должна содержать материнскую плату")
}

// TestCancelBuild проверяет отмену сборки через HTTP API
func TestCancelBuild(t *testing.T) {
	tests := []struct {
		name           string
		cancelTwice    bool
		wantHTTPStatus int
		wantErr        string
	}{
		{
			name:           "успешная отмена",
			cancelTwice:    false,
			wantHTTPStatus: http.StatusOK,
		},
		{
			name:           "повторная отмена",
			cancelTwice:    true,
			wantHTTPStatus: http.StatusUnprocessableEntity,
			wantErr:        "сборка уже отменена",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resetData(t)

			// Сначала создаём сборку
			createW := createBuildRequest(t, []string{motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4070})
			require.Equal(t, http.StatusCreated, createW.Code, "не удалось создать сборку")

			var createResp createBuildResponse
			require.NoError(t, json.NewDecoder(createW.Body).Decode(&createResp))

			// Первая отмена — всегда должна пройти
			cancelW := cancelBuildRequest(t, createResp.BuildUUID)
			require.Equal(t, http.StatusOK, cancelW.Code, "первая отмена должна пройти")
			assert.Equal(t, "application/json", cancelW.Header().Get("Content-Type"))

			var cancelResp cancelBuildResponse
			require.NoError(t, json.NewDecoder(cancelW.Body).Decode(&cancelResp))
			assert.Equal(t, createResp.BuildUUID, cancelResp.BuildUUID, "UUID сборки должен совпадать")
			assert.Equal(t, "cancelled", cancelResp.Status, "статус должен быть cancelled")

			// Проверяем, что reserved вернулся к 0.
			assert.Equal(t, 0, getReserved(t, motherboardZ790), "reserved материнской платы должен быть 0")
			assert.Equal(t, 0, getReserved(t, cpuIntelI7), "reserved процессора должен быть 0")
			assert.Equal(t, 0, getReserved(t, ramDDR5), "reserved RAM должен быть 0")
			assert.Equal(t, 0, getReserved(t, gpuRTX4070), "reserved GPU должен быть 0")

			// Проверяем статус в БД
			status := getBuildStatus(t, createResp.BuildUUID)
			assert.Equal(t, "cancelled", status, "статус в БД должен быть cancelled")

			if tt.cancelTwice {
				// Повторная отмена — должна вернуть ошибку
				cancelW2 := cancelBuildRequest(t, createResp.BuildUUID)
				assert.Equal(t, tt.wantHTTPStatus, cancelW2.Code, "повторная отмена должна вернуть 422")

				var errResp errorResponse
				require.NoError(t, json.NewDecoder(cancelW2.Body).Decode(&errResp))
				assert.Contains(t, errResp.Error, tt.wantErr)
			}
		})
	}
}

// TestCancelBuild_NotFound проверяет отмену несуществующей сборки
func TestCancelBuild_NotFound(t *testing.T) {
	resetData(t)

	w := cancelBuildRequest(t, "00000000-0000-0000-0000-000000000099")

	assert.Equal(t, http.StatusNotFound, w.Code, "несуществующая сборка должна вернуть 404")

	var resp errorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "сборка не найдена")
}

// TestFullLifecycle проверяет полный жизненный цикл: создание → проверка резервов → отмена → проверка освобождения
func TestFullLifecycle(t *testing.T) {
	resetData(t)

	components := []string{motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4070}

	// 1. Создаём сборку
	createW := createBuildRequest(t, components)
	require.Equal(t, http.StatusCreated, createW.Code)

	var createResp createBuildResponse
	require.NoError(t, json.NewDecoder(createW.Body).Decode(&createResp))
	assert.Equal(t, "reserved", createResp.Status)

	// 2. Проверяем, что компоненты зарезервированы
	for _, uuid := range components {
		assert.Equal(t, 1, getReserved(t, uuid),
			"reserved должен быть 1 после создания сборки для %s", uuid)
	}

	// 3. Отменяем сборку
	cancelW := cancelBuildRequest(t, createResp.BuildUUID)
	require.Equal(t, http.StatusOK, cancelW.Code)

	var cancelResp cancelBuildResponse
	require.NoError(t, json.NewDecoder(cancelW.Body).Decode(&cancelResp))
	assert.Equal(t, "cancelled", cancelResp.Status)

	// 4. Проверяем, что компоненты освобождены
	for _, uuid := range components {
		assert.Equal(t, 0, getReserved(t, uuid),
			"reserved должен быть 0 после отмены сборки для %s", uuid)
	}

	// 5. Проверяем статус в БД
	assert.Equal(t, "cancelled", getBuildStatus(t, createResp.BuildUUID))
}

// TestCreateBuild_ConcurrentReservation проверяет конкурентное резервирование
// компонента с ограниченным количеством (stock=1)
// 4 горутины пытаются зарезервировать RTX 4070 — но stock временно уменьшен до 1.
// Ровно одна горутина должна успеть, остальные получат ошибку
func TestCreateBuild_ConcurrentReservation(t *testing.T) {
	resetData(t)

	ctx := context.Background()

	// Уменьшаем stock RTX 4070 до 1 для теста конкурентности
	_, err := pool.Exec(ctx, "UPDATE components SET stock_quantity = 1 WHERE uuid = $1", gpuRTX4070)
	require.NoError(t, err, "не удалось обновить stock_quantity")

	const workers = 4

	var (
		wg           sync.WaitGroup
		successCount atomic.Int64
		errCount     atomic.Int64
	)

	// Все горутины пытаются создать Intel-сборку с RTX 4070 (stock=1)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			w := createBuildRequest(t, []string{motherboardZ790, cpuIntelI7, ramDDR5, gpuRTX4070})
			if w.Code == http.StatusCreated {
				successCount.Add(1)
			} else {
				errCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Ровно 1 горутина должна успеть зарезервировать RTX 4070.
	assert.Equal(t, int64(1), successCount.Load(),
		"ровно одна горутина должна успешно создать сборку")
	assert.Equal(t, int64(workers-1), errCount.Load(),
		"остальные горутины должны получить ошибку")

	// RTX 4070 полностью зарезервирована
	assert.Equal(t, 0, getAvailable(t, gpuRTX4070),
		"available RTX 4070 должен быть 0 после конкурентного резервирования")
}
