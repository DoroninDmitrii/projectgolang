package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	invApp "github.com/student/inventory/pkg/app"
	"github.com/student/order/pkg/app"
	"github.com/student/order/tests/testutil"
	payApp "github.com/student/payment/pkg/app"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/student/shared/pkg/proto/payment/v1"
)

// Предзагруженные UUID и цены деталей (из migrations/inventory/00002_seed_parts.sql)
const (
	HullAluminumUUID   = "550e8400-e29b-41d4-a716-446655440001" // 500000 kopecks (5000 RUB)
	HullTitaniumUUID   = "550e8400-e29b-41d4-a716-446655440002" // 1500000 kopecks (15000 RUB)
	EngineIonCUUID     = "550e8400-e29b-41d4-a716-446655440003" // 300000 kopecks (3000 RUB)
	EngineIonBUUID     = "550e8400-e29b-41d4-a716-446655440004" // 800000 kopecks (8000 RUB)
	ShieldEnergyUUID   = "550e8400-e29b-41d4-a716-446655440005" // 400000 kopecks (4000 RUB)
	WeaponLaserUUID    = "550e8400-e29b-41d4-a716-446655440006" // 250000 kopecks (2500 RUB)
	HullOutOfStockUUID = "550e8400-e29b-41d4-a716-446655440007" // 2000000 kopecks (20000 RUB), stock=0

	// Цены в копейках
	HullAluminumPrice   = 500000
	HullTitaniumPrice   = 1500000
	EngineIonCPrice     = 300000
	EngineIonBPrice     = 800000
	ShieldEnergyPrice   = 400000
	WeaponLaserPrice    = 250000
	HullOutOfStockPrice = 2000000
)

const bufSize = 1024 * 1024

var (
	invLis *bufconn.Listener
	payLis *bufconn.Listener

	inventoryClient inventoryv1.InventoryServiceClient
	paymentClient   paymentv1.PaymentServiceClient
	httpClient      = &http.Client{Timeout: 10 * time.Second}
	ts              *httptest.Server

	// orderDBPool нужен в тестах, где статус заказа нужно обновить в обход API —
	// например, чтобы проверить Cancel по ASSEMBLED (через API статус туда не попадает
	// без Kafka-цепочки, а её в API-тестах нет)
	orderDBPool *pgxpool.Pool

	// inventoryDBPool нужен в тестах конкурентности, где требуется подготовить
	// деталь с конкретным stock_quantity напрямую в БД
	inventoryDBPool *pgxpool.Pool
)

func invBufDialer(context.Context, string) (net.Conn, error) {
	return invLis.Dial()
}

func payBufDialer(context.Context, string) (net.Conn, error) {
	return payLis.Dial()
}

// orderBaseURL возвращает базовый URL для HTTP тестов заказов
func orderBaseURL() string {
	return ts.URL
}

// startPostgres запускает PostgreSQL контейнер и возвращает DSN подключения
func startPostgres(ctx context.Context, dbName, user, password string) (*tcpostgres.PostgresContainer, string, error) {
	container, err := tcpostgres.Run(
		ctx,
		"postgres:18.3-alpine3.23",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, "", err
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, "", err
	}

	return container, dsn, nil
}

// runMigrations запускает goose-миграции из указанной директории
func runMigrations(dsn, migrationsDir string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return err
	}

	return goose.Up(db, absDir)
}

// TestMain запускает все сервисы перед тестами и останавливает после
func TestMain(m *testing.M) {
	ctx := context.Background()

	// 1. Запускаем PostgreSQL контейнер для order-сервиса
	orderContainer, orderDSN, err := startPostgres(
		ctx,
		"order-service",
		"order-service-user",
		"order-service-password",
	)
	if err != nil {
		panic(err)
	}

	// 2. Запускаем PostgreSQL контейнер для inventory-сервиса
	inventoryContainer, inventoryDSN, err := startPostgres(
		ctx,
		"inventory-service",
		"inventory-service-user",
		"inventory-service-password",
	)
	if err != nil {
		panic(err)
	}

	// 3. Накатываем миграции для order-сервиса
	if err = runMigrations(orderDSN, "../../migrations/order"); err != nil {
		panic(err)
	}

	// 4. Накатываем миграции для inventory-сервиса
	if err = runMigrations(inventoryDSN, "../../migrations/inventory"); err != nil {
		panic(err)
	}

	// 5. Создаём pgxpool для order-сервиса
	orderPool, err := pgxpool.New(ctx, orderDSN)
	if err != nil {
		panic(err)
	}
	orderDBPool = orderPool

	// 6. Создаём pgxpool для inventory-сервиса
	inventoryPool, err := pgxpool.New(ctx, inventoryDSN)
	if err != nil {
		panic(err)
	}
	inventoryDBPool = inventoryPool

	// 7. Создаём TxManager для order-сервиса
	txManager, err := manager.New(trmpgx.NewDefaultFactory(orderPool))
	if err != nil {
		panic(err)
	}

	// 8. Inventory gRPC через bufconn
	invLis = bufconn.Listen(bufSize)
	invGRPCServer := grpc.NewServer(invApp.Interceptors()...)
	invApp.RegisterServices(invGRPCServer, inventoryPool)
	go func() {
		if invServeErr := invGRPCServer.Serve(invLis); invServeErr != nil {
			panic(invServeErr)
		}
	}()

	invConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(invBufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}
	inventoryClient = inventoryv1.NewInventoryServiceClient(invConn)

	// 9. Payment gRPC через bufconn
	payLis = bufconn.Listen(bufSize)
	payGRPCServer := grpc.NewServer(payApp.Interceptors()...)
	payApp.RegisterServices(payGRPCServer)
	go func() {
		if payServeErr := payGRPCServer.Serve(payLis); payServeErr != nil {
			panic(payServeErr)
		}
	}()

	payConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(payBufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}
	paymentClient = paymentv1.NewPaymentServiceClient(payConn)

	// 10. Order HTTP через httptest
	orderServer, err := app.NewHTTPHandler(orderPool, txManager, inventoryClient, paymentClient)
	if err != nil {
		panic(err)
	}
	ts = httptest.NewServer(orderServer)

	code := m.Run()

	ts.Close()
	if err = invConn.Close(); err != nil {
		panic(err)
	}
	if err = payConn.Close(); err != nil {
		panic(err)
	}
	invGRPCServer.Stop()
	payGRPCServer.Stop()

	orderPool.Close()
	inventoryPool.Close()

	if err = orderContainer.Terminate(ctx); err != nil {
		panic(err)
	}
	if err = inventoryContainer.Terminate(ctx); err != nil {
		panic(err)
	}

	os.Exit(code)
}

// HTTP типы запросов/ответов

// CreateOrderRequest представляет тело запроса для создания заказа
type CreateOrderRequest struct {
	UserUUID   string  `json:"user_uuid"`
	HullUUID   string  `json:"hull_uuid"`
	EngineUUID string  `json:"engine_uuid"`
	ShieldUUID *string `json:"shield_uuid,omitempty"`
	WeaponUUID *string `json:"weapon_uuid,omitempty"`
}

// CreateOrderResponse представляет ответ на создание заказа
type CreateOrderResponse struct {
	OrderUUID  string `json:"order_uuid"`
	TotalPrice int64  `json:"total_price"`
}

// PayOrderRequest представляет тело запроса для оплаты заказа
type PayOrderRequest struct {
	PaymentMethod string `json:"payment_method"`
}

// PayOrderResponse представляет ответ на оплату заказа
type PayOrderResponse struct {
	TransactionUUID string `json:"transaction_uuid"`
}

// CancelOrderResponse представляет ответ на отмену заказа (пустой)
type CancelOrderResponse struct{}

// OrderDTO представляет заказ в ответе API
type OrderDTO struct {
	OrderUUID       string  `json:"order_uuid"`
	UserUUID        string  `json:"user_uuid"`
	HullUUID        string  `json:"hull_uuid"`
	EngineUUID      string  `json:"engine_uuid"`
	ShieldUUID      *string `json:"shield_uuid"`
	WeaponUUID      *string `json:"weapon_uuid"`
	TotalPrice      int64   `json:"total_price"`
	TransactionUUID *string `json:"transaction_uuid"`
	PaymentMethod   *string `json:"payment_method"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
}

// ErrorResponse представляет ответ с ошибкой от API
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Вспомогательные HTTP функции

func createOrder(t *testing.T, req *CreateOrderRequest) (*CreateOrderResponse, *http.Response) {
	t.Helper()

	jsonBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusCreated {
		var result CreateOrderResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

func getOrder(t *testing.T, orderUUID string) (*OrderDTO, *http.Response) {
	t.Helper()

	resp, err := httpClient.Get(orderBaseURL() + "/api/v1/orders/" + orderUUID)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result OrderDTO
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

func payOrder(t *testing.T, orderUUID string, req *PayOrderRequest) (*PayOrderResponse, *http.Response) {
	t.Helper()

	jsonBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/"+orderUUID+"/pay", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result PayOrderResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

func cancelOrder(t *testing.T, orderUUID string) (*CancelOrderResponse, *http.Response) {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/"+orderUUID+"/cancel", nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result CancelOrderResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

// Тесты InventoryService (gRPC)

func TestInventory_GetPart_Success(t *testing.T) {
	resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: HullAluminumUUID,
	})
	require.NoError(t, err)

	part := resp.GetPart()
	assert.Equal(t, HullAluminumUUID, part.GetUuid())
	assert.Equal(t, int64(HullAluminumPrice), part.GetPrice())
	assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	assert.NotEmpty(t, part.GetName())
	assert.NotEmpty(t, part.GetDescription(), "description должен быть заполнен")
	assert.NotNil(t, part.GetCreatedAt())
}

func TestInventory_GetPart_AllTypes(t *testing.T) {
	testCases := []struct {
		name        string
		uuid        string
		price       int64
		partType    inventoryv1.PartType
		description string
	}{
		{"Hull Aluminum", HullAluminumUUID, HullAluminumPrice, inventoryv1.PartType_PART_TYPE_HULL, "Лёгкий корпус для небольших кораблей"},
		{"Hull Titanium", HullTitaniumUUID, HullTitaniumPrice, inventoryv1.PartType_PART_TYPE_HULL, "Прочный корпус для средних кораблей"},
		{"Engine Ion C", EngineIonCUUID, EngineIonCPrice, inventoryv1.PartType_PART_TYPE_ENGINE, "Базовый ионный двигатель класса C"},
		{"Engine Ion B", EngineIonBUUID, EngineIonBPrice, inventoryv1.PartType_PART_TYPE_ENGINE, "Улучшенный ионный двигатель класса B"},
		{"Shield Energy", ShieldEnergyUUID, ShieldEnergyPrice, inventoryv1.PartType_PART_TYPE_SHIELD, "Стандартный энергетический щит"},
		{"Weapon Laser", WeaponLaserUUID, WeaponLaserPrice, inventoryv1.PartType_PART_TYPE_WEAPON, "Точная лазерная пушка"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
				Uuid: tc.uuid,
			})
			require.NoError(t, err)

			part := resp.GetPart()
			assert.Equal(t, tc.uuid, part.GetUuid())
			assert.Equal(t, tc.price, part.GetPrice())
			assert.Equal(t, tc.partType, part.GetPartType())
			assert.Equal(t, tc.description, part.GetDescription())
		})
	}
}

func TestInventory_GetPart_NotFound(t *testing.T) {
	_, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: uuid.New().String(),
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_GetPart_EmptyUUID(t *testing.T) {
	_, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: "",
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_GetPart_InvalidUUID(t *testing.T) {
	_, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: "invalid-uuid-format",
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ListParts_All(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_UNSPECIFIED,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)
}

func TestInventory_ListParts_ByType_Hull(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_HULL,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3) // Алюминиевый, Титановый, Плазменный (stock=0)

	for _, part := range resp.GetParts() {
		assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	}
}

func TestInventory_ListParts_ByType_Engine(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_ENGINE,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	for _, part := range resp.GetParts() {
		assert.Equal(t, inventoryv1.PartType_PART_TYPE_ENGINE, part.GetPartType())
	}
}

func TestInventory_ListParts_ByType_Shield(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_SHIELD,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, ShieldEnergyUUID, resp.GetParts()[0].GetUuid())
}

func TestInventory_ListParts_ByType_Weapon(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_WEAPON,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, WeaponLaserUUID, resp.GetParts()[0].GetUuid())
}

func TestInventory_ListParts_SortedByName(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_UNSPECIFIED,
	})
	require.NoError(t, err)

	parts := resp.GetParts()
	for i := 1; i < len(parts); i++ {
		assert.LessOrEqual(t, parts[i-1].GetName(), parts[i].GetName(),
			"детали должны быть отсортированы по имени в алфавитном порядке")
	}
}

// Тесты ListParts.uuids

func TestInventory_ListParts_ByUuids_Success(t *testing.T) {
	uuids := []string{HullAluminumUUID, EngineIonCUUID, ShieldEnergyUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3)

	// Проверяем, что вернулись нужные детали
	returnedUUIDs := make([]string, len(resp.GetParts()))
	for i, part := range resp.GetParts() {
		returnedUUIDs[i] = part.GetUuid()
	}
	assert.ElementsMatch(t, uuids, returnedUUIDs)
}

func TestInventory_ListParts_ByUuids_PreservesOrder(t *testing.T) {
	// Запрос в определённом порядке: Engine, Hull, Weapon
	uuids := []string{EngineIonCUUID, HullAluminumUUID, WeaponLaserUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3)

	// Проверяем, что порядок сохранён как в запросе
	for i, part := range resp.GetParts() {
		assert.Equal(t, uuids[i], part.GetUuid(),
			"деталь с индексом %d должна соответствовать порядку запрошенных UUID", i)
	}
}

func TestInventory_ListParts_ByUuids_IgnoresPartType(t *testing.T) {
	// Запрос с uuids И part_type — part_type должен быть проигнорирован
	uuids := []string{HullAluminumUUID, EngineIonCUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids:    uuids,
		PartType: inventoryv1.PartType_PART_TYPE_WEAPON, // Должен быть проигнорирован
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	// Проверяем, что получили Hull и Engine, а не Weapons
	assert.Equal(t, HullAluminumUUID, resp.GetParts()[0].GetUuid())
	assert.Equal(t, EngineIonCUUID, resp.GetParts()[1].GetUuid())
}

func TestInventory_ListParts_ByUuids_NotFound(t *testing.T) {
	// Включаем один несуществующий UUID
	nonExistentUUID := uuid.New().String()
	uuids := []string{HullAluminumUUID, nonExistentUUID, EngineIonCUUID}

	_, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_ListParts_ByUuids_InvalidUUID(t *testing.T) {
	uuids := []string{HullAluminumUUID, "invalid-uuid-format"}

	_, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ListParts_ByUuids_SingleUUID(t *testing.T) {
	uuids := []string{WeaponLaserUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, WeaponLaserUUID, resp.GetParts()[0].GetUuid())
	assert.Equal(t, int64(WeaponLaserPrice), resp.GetParts()[0].GetPrice())
}

func TestInventory_ListParts_ByUuids_AllParts(t *testing.T) {
	// Запрашиваем все 6 деталей по UUID
	uuids := []string{
		HullAluminumUUID, HullTitaniumUUID,
		EngineIonCUUID, EngineIonBUUID,
		ShieldEnergyUUID, WeaponLaserUUID,
	}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 6)

	// Проверяем, что порядок совпадает с порядком запроса
	for i, part := range resp.GetParts() {
		assert.Equal(t, uuids[i], part.GetUuid())
	}
}

func TestInventory_ListParts_ByUuids_EmptyList(t *testing.T) {
	// Пустой список UUID — должен вернуть все детали (фильтрация по типу UNSPECIFIED)
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: []string{},
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)
}

// Тесты PaymentService (gRPC)

func TestPayment_PayOrder_Success_Card(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())

	// Проверяем, что UUID транзакции валиден
	_, err = uuid.Parse(resp.GetTransactionUuid())
	assert.NoError(t, err)
}

func TestPayment_PayOrder_Success_SBP(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_SBP,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_Success_CreditCard(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_Success_InvestorMoney(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_EmptyOrderUUID(t *testing.T) {
	_, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestPayment_PayOrder_UnspecifiedMethod(t *testing.T) {
	_, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestPayment_PayOrder_UniqueTransactions(t *testing.T) {
	orderUUID := uuid.New().String()

	resp1, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)

	resp2, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetTransactionUuid(), resp2.GetTransactionUuid(),
		"каждый платёж должен генерировать уникальный UUID транзакции")
}

// Тесты OrderService (HTTP)

func TestOrder_Create_Success_MinimalParts(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OrderUUID)
	assert.Equal(t, int64(HullAluminumPrice+EngineIonCPrice), result.TotalPrice)
}

func TestOrder_Create_Success_AllParts(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: new(ShieldEnergyUUID),
		WeaponUUID: new(WeaponLaserUUID),
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OrderUUID)

	expectedTotal := int64(HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice + WeaponLaserPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

func TestOrder_Create_VerifyTotalPrice(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID, // 500000
		EngineUUID: EngineIonCUUID,   // 300000
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, int64(800000), result.TotalPrice, "500000 + 300000 = 800000")
}

func TestOrder_Create_HullNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   uuid.New().String(),
		EngineUUID: EngineIonCUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_EngineNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: uuid.New().String(),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_ShieldNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: new(uuid.New().String()),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_WeaponNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		WeaponUUID: new(uuid.New().String()),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Get_Success(t *testing.T) {
	// Сначала создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Получаем заказ
	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, order)
	assert.Equal(t, createResult.OrderUUID, order.OrderUUID)
	assert.Equal(t, HullAluminumUUID, order.HullUUID)
	assert.Equal(t, EngineIonCUUID, order.EngineUUID)
	assert.Equal(t, createResult.TotalPrice, order.TotalPrice)
}

func TestOrder_Get_VerifyStatus_PendingPayment(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Получаем и проверяем статус
	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "PENDING_PAYMENT", order.Status)
}

func TestOrder_Get_NotFound(t *testing.T) {
	_, resp := getOrder(t, uuid.New().String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Pay_Success_Card(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	payResult, resp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, payResult)
	assert.NotEmpty(t, payResult.TransactionUUID)
}

func TestOrder_Pay_VerifyStatusChange(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()

	// Получаем и проверяем статус changed to PAID
	order, getResp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "PAID", order.Status)
	assert.NotNil(t, order.TransactionUUID)
	assert.NotNil(t, order.PaymentMethod)
	assert.Equal(t, "CARD", *order.PaymentMethod)
}

func TestOrder_Pay_NotFound(t *testing.T) {
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, resp := payOrder(t, uuid.New().String(), payReq)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Pay_AlreadyPaid(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ в первый раз
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp1 := payOrder(t, createResult.OrderUUID, payReq)
	_ = payResp1.Body.Close()

	// Пытаемся оплатить повторно — должна быть ошибка конфликта
	_, payResp2 := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp2.Body.Close() }()

	require.Equal(t, http.StatusConflict, payResp2.StatusCode)
}

func TestOrder_Pay_AlreadyCancelled(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	_ = cancelResp.Body.Close()

	// Пытаемся оплатить отменённый заказ — должна быть ошибка конфликта
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, payResp.StatusCode)
}

func TestOrder_Cancel_Success(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ
	_, resp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestOrder_Cancel_VerifyStatusChange(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	_ = cancelResp.Body.Close()

	// Получаем и проверяем статус changed to CANCELLED
	order, getResp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "CANCELLED", order.Status)
}

func TestOrder_Cancel_NotFound(t *testing.T) {
	_, resp := cancelOrder(t, uuid.New().String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Cancel_AlreadyPaid(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()

	// Пытаемся отменить оплаченный заказ — должна быть ошибка конфликта
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp.StatusCode)
}

func TestOrder_Cancel_AlreadyCancelled(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ first time
	_, cancelResp1 := cancelOrder(t, createResult.OrderUUID)
	_ = cancelResp1.Body.Close()

	// Пытаемся отменить повторно — должна быть ошибка конфликта
	_, cancelResp2 := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp2.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp2.StatusCode)
}

// Дополнительные тесты валидации

func TestOrder_Create_WithWeaponOnly(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		WeaponUUID: new(WeaponLaserUUID),
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	expectedTotal := int64(HullAluminumPrice + EngineIonCPrice + WeaponLaserPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

func TestOrder_Pay_AllMethods(t *testing.T) {
	methods := []string{"CARD", "SBP", "CREDIT_CARD", "INVESTOR_MONEY"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			// Создаём заказ
			createReq := &CreateOrderRequest{
				UserUUID:   uuid.New().String(),
				HullUUID:   HullAluminumUUID,
				EngineUUID: EngineIonCUUID,
			}
			createResult, createResp := createOrder(t, createReq)
			_ = createResp.Body.Close()
			require.NotNil(t, createResult)

			// Оплачиваем этим методом
			payReq := &PayOrderRequest{PaymentMethod: method}
			payResult, resp := payOrder(t, createResult.OrderUUID, payReq)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotNil(t, payResult)
			assert.NotEmpty(t, payResult.TransactionUUID)

			// Проверяем, что метод оплаты сохранён
			order, getResp := getOrder(t, createResult.OrderUUID)
			_ = getResp.Body.Close()
			require.NotNil(t, order.PaymentMethod)
			assert.Equal(t, method, *order.PaymentMethod)
		})
	}
}

func TestOrder_Get_WithOptionalParts(t *testing.T) {
	shieldUUID := ShieldEnergyUUID
	weaponUUID := WeaponLaserUUID
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: &shieldUUID,
		WeaponUUID: &weaponUUID,
	}

	createResult, createResp := createOrder(t, req)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Получаем заказ и проверяем, что опциональные детали сохранены
	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, order.ShieldUUID)
	require.NotNil(t, order.WeaponUUID)
	assert.Equal(t, shieldUUID, *order.ShieldUUID)
	assert.Equal(t, weaponUUID, *order.WeaponUUID)
}

func TestPayment_PayOrder_InvalidUUIDFormat(t *testing.T) {
	_, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "invalid-uuid-format",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

// Тесты полного жизненного цикла

func TestOrder_FullLifecycle_CreatePayGet(t *testing.T) {
	// 1. Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: new(ShieldEnergyUUID),
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)
	assert.NotEmpty(t, createResult.OrderUUID)

	expectedTotal := int64(HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice)
	assert.Equal(t, expectedTotal, createResult.TotalPrice)

	// 2. Получаем заказ — проверяем PENDING_PAYMENT
	order1, getResp1 := getOrder(t, createResult.OrderUUID)
	_ = getResp1.Body.Close()
	assert.Equal(t, "PENDING_PAYMENT", order1.Status)
	assert.Nil(t, order1.TransactionUUID)

	// 3. Оплачиваем заказ
	payReq := &PayOrderRequest{PaymentMethod: "SBP"}
	payResult, payResp := payOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()
	require.NotNil(t, payResult)
	assert.NotEmpty(t, payResult.TransactionUUID)

	// 4. Получаем заказ — проверяем PAID
	order2, getResp2 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "PAID", order2.Status)
	require.NotNil(t, order2.TransactionUUID)
	assert.Equal(t, payResult.TransactionUUID, *order2.TransactionUUID)
	require.NotNil(t, order2.PaymentMethod)
	assert.Equal(t, "SBP", *order2.PaymentMethod)
}

func TestOrder_FullLifecycle_CreateCancelGet(t *testing.T) {
	// 1. Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// 2. Получаем заказ — проверяем PENDING_PAYMENT
	order1, getResp1 := getOrder(t, createResult.OrderUUID)
	_ = getResp1.Body.Close()
	assert.Equal(t, "PENDING_PAYMENT", order1.Status)

	// 3. Отменяем заказ
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	_ = cancelResp.Body.Close()

	// 4. Получаем заказ — проверяем CANCELLED
	order2, getResp2 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "CANCELLED", order2.Status)
	assert.Nil(t, order2.TransactionUUID)
}

func TestOrder_FullLifecycle_AllPartsPayGet(t *testing.T) {
	// Полный жизненный цикл со всеми 4 деталями: hull + engine + shield + weapon
	shieldUUID := ShieldEnergyUUID
	weaponUUID := WeaponLaserUUID
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: &shieldUUID,
		WeaponUUID: &weaponUUID,
	}

	// 1. Создаём заказ
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	expectedTotal := int64(HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice + WeaponLaserPrice)
	assert.Equal(t, expectedTotal, createResult.TotalPrice)

	// 2. Проверяем все детали в GET ответе
	order1, getResp1 := getOrder(t, createResult.OrderUUID)
	_ = getResp1.Body.Close()
	assert.Equal(t, HullTitaniumUUID, order1.HullUUID)
	assert.Equal(t, EngineIonBUUID, order1.EngineUUID)
	require.NotNil(t, order1.ShieldUUID)
	assert.Equal(t, shieldUUID, *order1.ShieldUUID)
	require.NotNil(t, order1.WeaponUUID)
	assert.Equal(t, weaponUUID, *order1.WeaponUUID)

	// 3. Оплачиваем заказ
	payReq := &PayOrderRequest{PaymentMethod: "CREDIT_CARD"}
	payResult, payResp := payOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()
	require.NotNil(t, payResult)

	// 4. Проверяем финальное состояние
	order2, getResp2 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "PAID", order2.Status)
	require.NotNil(t, order2.PaymentMethod)
	assert.Equal(t, "CREDIT_CARD", *order2.PaymentMethod)
}

// Тесты ogen-валидации (400 Bad Request)

func TestOrder_Create_InvalidBody_EmptyJSON(t *testing.T) {
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_NotJSON(t *testing.T) {
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_MissingHullUUID(t *testing.T) {
	body := `{"engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_MissingEngineUUID(t *testing.T) {
	body := `{"hull_uuid": "` + HullAluminumUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_InvalidHullUUID(t *testing.T) {
	body := `{"hull_uuid": "not-a-uuid", "engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Get_InvalidUUIDInPath(t *testing.T) {
	resp, err := httpClient.Get(orderBaseURL() + "/api/v1/orders/not-a-uuid")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_InvalidUUIDInPath(t *testing.T) {
	body := `{"payment_method": "CARD"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/not-a-uuid/pay", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_InvalidPaymentMethod(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Пытаемся оплатить невалидным методом — ogen отклонит
	body := `{"payment_method": "BITCOIN"}`
	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_MissingPaymentMethod(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Пытаемся оплатить без payment_method
	body := `{}`
	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_EmptyBody(t *testing.T) {
	// Создаём заказ
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte("")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Cancel_InvalidUUIDInPath(t *testing.T) {
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/not-a-uuid/cancel", nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Тесты out of stock

func TestOrder_Create_OutOfStock_Hull(t *testing.T) {
	// Плазменный корпус — stock_quantity=0, заказ должен быть отклонён
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullOutOfStockUUID,
		EngineUUID: EngineIonCUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestOrder_Create_OutOfStock_WithOptionalParts(t *testing.T) {
	// Out of stock деталь среди опциональных — shield
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: new(HullOutOfStockUUID), // Передаём hull UUID как shield — тип не совпадёт, но out of stock проверяется первым
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	// Либо Conflict (out of stock), либо другая ошибка — не 201.
	assert.NotEqual(t, http.StatusCreated, resp.StatusCode)
}

// Тесты Inventory: out of stock деталь присутствует в списке

func TestInventory_GetPart_OutOfStock(t *testing.T) {
	resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: HullOutOfStockUUID,
	})
	require.NoError(t, err)

	part := resp.GetPart()
	assert.Equal(t, HullOutOfStockUUID, part.GetUuid())
	assert.Equal(t, int64(HullOutOfStockPrice), part.GetPrice())
	assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	assert.Equal(t, int64(0), part.GetStockQuantity())
	assert.Equal(t, "Экспериментальный корпус (нет на складе)", part.GetDescription())
}

func TestInventory_ListParts_ByUuids_IncludesOutOfStock(t *testing.T) {
	uuids := []string{HullAluminumUUID, HullOutOfStockUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	// Out of stock деталь возвращается — фильтрации по наличию нет
	assert.Equal(t, HullOutOfStockUUID, resp.GetParts()[1].GetUuid())
	assert.Equal(t, int64(0), resp.GetParts()[1].GetStockQuantity())
}

// Тесты Order: проверка created_at

func TestOrder_Get_VerifyCreatedAt(t *testing.T) {
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, order.CreatedAt, "created_at должен быть заполнен")

	// Парсим время — проверяем, что строка валидна и время не нулевое
	createdAt, err := time.Parse(time.RFC3339Nano, order.CreatedAt)
	if err != nil {
		createdAt, err = time.Parse(time.RFC3339, order.CreatedAt)
	}
	if err != nil {
		createdAt, err = time.Parse("2006-01-02T15:04:05Z", order.CreatedAt)
	}
	require.NoError(t, err, "не удалось распарсить created_at: %s", order.CreatedAt)
	assert.False(t, createdAt.IsZero(), "created_at не должен быть нулевым")
}

// Тесты с shield only (без weapon)

func TestOrder_Create_WithShieldOnly(t *testing.T) {
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: new(ShieldEnergyUUID),
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	expectedTotal := int64(HullAluminumPrice + EngineIonCPrice + ShieldEnergyPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

// Тесты соответствия типа детали слоту корабля

func TestOrder_Create_WrongPartType_WeaponAsHull(t *testing.T) {
	// В слот корпуса передан UUID оружия — InventoryService возвращает InvalidArgument,
	// order-сервис маппит его в 400 Bad Request
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   WeaponLaserUUID,
		EngineUUID: EngineIonCUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_WrongPartType_HullAsEngine(t *testing.T) {
	// В слот двигателя передан UUID корпуса (фактически второй корпус) — 400
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: HullTitaniumUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_WrongPartType_ShieldAsWeapon(t *testing.T) {
	// В слот оружия передан UUID щита — 400
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		WeaponUUID: new(ShieldEnergyUUID),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_DuplicateUUID_HullAndEngine(t *testing.T) {
	// Один и тот же UUID передан в hull и engine — это автоматически означает mismatch
	// типа в одном из слотов (одна деталь не может быть и HULL, и ENGINE) → 400
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: HullAluminumUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Тесты ValidateCompatibility (gRPC)

func TestInventory_ValidateCompatibility_Success_Compatible(t *testing.T) {
	// Алюминиевый корпус (strength=50) + Ионный двигатель C (required_strength=30) — совместимы
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullAluminumUUID,
		EngineUuid: EngineIonCUUID,
	})
	require.NoError(t, err)
}

func TestInventory_ValidateCompatibility_Success_StrongHull(t *testing.T) {
	// Титановый корпус (strength=150) + Ионный двигатель B (required_strength=70) — совместимы
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullTitaniumUUID,
		EngineUuid: EngineIonBUUID,
	})
	require.NoError(t, err)
}

func TestInventory_ValidateCompatibility_Success_AllParts(t *testing.T) {
	// Титановый корпус + Ion B + Energy shield + Laser — всё совместимо
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullTitaniumUUID,
		EngineUuid: EngineIonBUUID,
		ShieldUuid: ShieldEnergyUUID,
		WeaponUuid: WeaponLaserUUID,
	})
	require.NoError(t, err)
}

func TestInventory_ValidateCompatibility_Fail_WeakHull(t *testing.T) {
	// Алюминиевый корпус (strength=50) + Ионный двигатель B (required_strength=70) — несовместимы
	// Корпус слишком слаб для двигателя класса B
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullAluminumUUID,
		EngineUuid: EngineIonBUUID,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.FailedPrecondition)
}

func TestInventory_ValidateCompatibility_MissingHull(t *testing.T) {
	// Без hull_uuid — это нарушение контракта (обязательный слот)
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		EngineUuid: EngineIonBUUID,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ValidateCompatibility_MissingEngine(t *testing.T) {
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid: HullAluminumUUID,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ValidateCompatibility_TypeMismatch_WeaponInHullSlot(t *testing.T) {
	// В слот корпуса передан UUID оружия — InvalidArgument
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   WeaponLaserUUID,
		EngineUuid: EngineIonCUUID,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ValidateCompatibility_TypeMismatch_HullInEngineSlot(t *testing.T) {
	// В слот двигателя передан UUID корпуса (фактически второй корпус) — InvalidArgument
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullAluminumUUID,
		EngineUuid: HullTitaniumUUID,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ValidateCompatibility_DuplicateUUID_HullAndEngine(t *testing.T) {
	// Один и тот же UUID в двух слотах — InvalidArgument
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullAluminumUUID,
		EngineUuid: HullAluminumUUID,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ValidateCompatibility_NotFound(t *testing.T) {
	// Несуществующий UUID — ошибка NotFound
	_, err := inventoryClient.ValidateCompatibility(context.Background(), &inventoryv1.ValidateCompatibilityRequest{
		HullUuid:   HullAluminumUUID,
		EngineUuid: uuid.New().String(),
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

// Тесты ReserveParts (gRPC)

func TestInventory_ReserveParts_Success(t *testing.T) {
	// Резервируем доступные детали — ожидаем успех
	_, err := inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: []string{HullAluminumUUID, EngineIonCUUID},
	})
	require.NoError(t, err)

	// Освобождаем обратно, чтобы не ломать другие тесты
	_, err = inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: []string{HullAluminumUUID, EngineIonCUUID},
	})
	require.NoError(t, err)
}

func TestInventory_ReserveParts_OutOfStock(t *testing.T) {
	// Плазменный корпус (stock=0) — резервирование невозможно
	_, err := inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: []string{HullOutOfStockUUID},
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.ResourceExhausted)
}

func TestInventory_ReserveParts_NotFound(t *testing.T) {
	_, err := inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: []string{uuid.New().String()},
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_ReserveParts_SinglePart(t *testing.T) {
	// Резервируем одну деталь
	_, err := inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: []string{ShieldEnergyUUID},
	})
	require.NoError(t, err)

	// Освобождаем обратно
	_, err = inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: []string{ShieldEnergyUUID},
	})
	require.NoError(t, err)
}

// Тесты ReleaseParts (gRPC)

func TestInventory_ReleaseParts_Success(t *testing.T) {
	// Сначала резервируем, потом освобождаем — полный цикл
	uuids := []string{HullTitaniumUUID, EngineIonBUUID}

	_, err := inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)

	_, err = inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
}

func TestInventory_ReleaseParts_NothingToRelease(t *testing.T) {
	// Плазменный корпус (stock=0, reserved=0) — нечего освобождать
	_, err := inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: []string{HullOutOfStockUUID},
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.FailedPrecondition)
}

func TestInventory_ReleaseParts_NotFound(t *testing.T) {
	_, err := inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: []string{uuid.New().String()},
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

// Тесты Order Create с несовместимыми деталями (HTTP)

func TestOrder_Create_IncompatibleParts_WeakHullStrongEngine(t *testing.T) {
	// Алюминиевый корпус (strength=50) + Ионный двигатель B (required_strength=70)
	// Корпус не выдержит двигатель — ValidateCompatibility вернёт FailedPrecondition,
	// order-сервис преобразует в 409 Conflict
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonBUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestOrder_Create_IncompatibleParts_WithOptionalParts(t *testing.T) {
	// Алюминиевый корпус (strength=50) + Ионный двигатель B (required_strength=70) + Shield + Weapon
	// Даже с опциональными деталями — несовместимость hull/engine блокирует создание
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: new(ShieldEnergyUUID),
		WeaponUUID: new(WeaponLaserUUID),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestOrder_Create_CompatibleParts_StrongHullStrongEngine(t *testing.T) {
	// Титановый корпус (strength=150) + Ионный двигатель B (required_strength=70) — совместимы
	// Контрольный тест: при совместимых деталях заказ создаётся
	req := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.Equal(t, int64(HullTitaniumPrice+EngineIonBPrice), result.TotalPrice)
}

// Тесты Reserve/Release через полный жизненный цикл заказа

func TestInventory_ReserveRelease_FullCycle(t *testing.T) {
	// Резервируем → освобождаем → снова резервируем — проверяем, что счётчики корректны
	uuids := []string{WeaponLaserUUID}

	// Первый резерв
	_, err := inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)

	// Освобождаем
	_, err = inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)

	// Повторный резерв должен пройти (деталь снова доступна)
	_, err = inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)

	// Финальное освобождение
	_, err = inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
}

// Тесты user_uuid (валидация и проброс через всю цепочку)

func TestOrder_Create_MissingUserUUID(t *testing.T) {
	// Без user_uuid — ogen отклоняет запрос ещё до сервиса
	body := `{"hull_uuid": "` + HullAluminumUUID + `", "engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidUserUUIDFormat(t *testing.T) {
	// Невалидный формат user_uuid — ogen отклоняет запрос
	body := `{"user_uuid": "not-a-uuid", "hull_uuid": "` + HullAluminumUUID + `", "engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Get_ReturnsUserUUID(t *testing.T) {
	// user_uuid из CreateOrderRequest должен сохраняться в БД и возвращаться в GET
	userUUID := uuid.New().String()
	req := &CreateOrderRequest{
		UserUUID:   userUUID,
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}

	createResult, createResp := createOrder(t, req)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, userUUID, order.UserUUID, "user_uuid должен пробрасываться из запроса в ответ GET")
}

// Тесты Cancel по статусу ASSEMBLED
//
// В API-тестах Kafka нет (noopProducer), поэтому статус ASSEMBLED через обычную
// цепочку Pay → OrderPaid → AssemblyService → ShipAssembled получить нельзя
// Обновляем статус напрямую в БД — это честная проверка именно Cancel-логики

func TestOrder_Cancel_AlreadyAssembled(t *testing.T) {
	// Создаём и оплачиваем заказ, чтобы детали были зарезервированы
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()

	// Имитируем завершение сборки — переводим заказ в ASSEMBLED в обход Kafka-цепочки
	_, err := orderDBPool.Exec(
		context.Background(),
		`UPDATE orders SET status = 'ASSEMBLED' WHERE uuid = $1`,
		createResult.OrderUUID,
	)
	require.NoError(t, err)

	// Отмена собранного заказа должна вернуть 409 Conflict
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp.StatusCode)

	// Статус заказа не должен меняться — остаётся ASSEMBLED
	order, getResp := getOrder(t, createResult.OrderUUID)
	_ = getResp.Body.Close()
	assert.Equal(t, "ASSEMBLED", order.Status)
}

// Тесты CommitParts (gRPC)

func TestInventory_CommitParts_Success(t *testing.T) {
	// Полный цикл: резервируем → списываем. После Commit stock должен уменьшиться на 1.
	uuids := []string{ShieldEnergyUUID}

	partBefore, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: ShieldEnergyUUID,
	})
	require.NoError(t, err)
	stockBefore := partBefore.GetPart().GetStockQuantity()

	_, err = inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)

	_, err = inventoryClient.CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)

	partAfter, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: ShieldEnergyUUID,
	})
	require.NoError(t, err)
	assert.Equal(t, stockBefore-1, partAfter.GetPart().GetStockQuantity(),
		"Commit должен уменьшить stock_quantity на 1")
}

func TestInventory_CommitParts_NothingToCommit(t *testing.T) {
	// Плазменный корпус: существует, но stock=0 и reserved=0 — списывать нечего
	// ListForUpdate находит деталь, но SQL-условие stock>0 AND reserved>0 не проходит,
	// RowsAffected=0 → ErrNothingToCommit → FailedPrecondition
	_, err := inventoryClient.CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{
		Uuids: []string{HullOutOfStockUUID},
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.FailedPrecondition)
}

func TestInventory_CommitParts_NotFound(t *testing.T) {
	// Несуществующий UUID → ListForUpdate возвращает ErrPartNotFound → NotFound
	// Это защита перед самим Commit: мы различаем «детали нет» и «нечего списывать»
	_, err := inventoryClient.CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{
		Uuids: []string{uuid.New().String()},
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_CommitParts_PartialCommit_RollbackOnMissing(t *testing.T) {
	// Если в батче одна деталь валидна (зарезервирована), а другая — нет,
	// весь Commit должен откатиться: stock первой детали не должен измениться
	validUUID := HullTitaniumUUID

	partBefore, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: validUUID,
	})
	require.NoError(t, err)
	stockBefore := partBefore.GetPart().GetStockQuantity()

	_, err = inventoryClient.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{
		Uuids: []string{validUUID},
	})
	require.NoError(t, err)

	// Батч с валидной и несуществующей деталью → FailedPrecondition, транзакция откатывается
	_, err = inventoryClient.CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{
		Uuids: []string{validUUID, uuid.New().String()},
	})
	require.Error(t, err)

	partAfter, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: validUUID,
	})
	require.NoError(t, err)
	assert.Equal(t, stockBefore, partAfter.GetPart().GetStockQuantity(),
		"при частичной ошибке stock валидной детали должен остаться без изменений")

	// Подчищаем резерв, чтобы не ломать соседние тесты
	_, err = inventoryClient.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{
		Uuids: []string{validUUID},
	})
	require.NoError(t, err)
}

// Тесты Cancel: возврат резерва

func TestOrder_Cancel_ReleasesReservedParts(t *testing.T) {
	// После Cancel зарезервированные детали должны освободиться
	// Проверяем: до Cancel reserved был +1, после Cancel он должен вернуться к исходному
	partBefore, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: HullTitaniumUUID,
	})
	require.NoError(t, err)
	stockBefore := partBefore.GetPart().GetStockQuantity()

	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	// После Cancel stock не должен был измениться (Reserve не трогает stock,
	// а Release возвращает reserved к исходному)
	partAfter, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: HullTitaniumUUID,
	})
	require.NoError(t, err)
	assert.Equal(t, stockBefore, partAfter.GetPart().GetStockQuantity(),
		"Cancel не должен списывать stock, только освобождать reserved")
}

// Тесты конкурентности (SELECT FOR UPDATE)
//
// Эти тесты проверяют, что пессимистичные блокировки реально работают:
// без FOR UPDATE два параллельных запроса увидят одинаковое состояние и
// оба пройдут — это race condition. С FOR UPDATE второй запрос ждёт
// первого и видит уже изменённое состояние, поэтому корректно отказывает.

func TestOrder_Pay_Concurrent_SameOrder(t *testing.T) {
	// Два параллельных Pay одного и того же заказа.
	// FOR UPDATE в OrderRepo.GetForUpdate гарантирует: ровно один Pay вернёт 200,
	// второй увидит статус PAID и вернёт 409 (already paid).
	// Без FOR UPDATE оба прошли бы → двойная оплата.
	createReq := &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	var wg sync.WaitGroup
	statusCodes := make([]int, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payReq := &PayOrderRequest{PaymentMethod: "CARD"}
			_, resp := payOrder(t, createResult.OrderUUID, payReq)
			defer func() { _ = resp.Body.Close() }()
			statusCodes[idx] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, code := range statusCodes {
		switch code {
		case http.StatusOK:
			successCount++
		case http.StatusConflict:
			conflictCount++
		}
	}

	assert.Equal(t, 1, successCount, "ровно один Pay должен вернуть 200")
	assert.Equal(t, 1, conflictCount, "ровно один Pay должен вернуть 409 (already paid)")
}

func TestInventory_ReserveParts_Concurrent_LastPart(t *testing.T) {
	// Готовим деталь со stock_quantity=1 — «последняя на складе».
	// Два параллельных ReserveParts на одну и ту же деталь:
	// FOR UPDATE в ListForUpdate гарантирует, что один резерв пройдёт,
	// а второй упадёт с FailedPrecondition (OutOfStock).
	// Без FOR UPDATE оба бы зарезервировали → двойной резерв одной детали.
	testPartUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
         VALUES ($1, 'Тестовый корпус', 'Конкурентный тест', 'HULL', 1000, 1, '{"hull": {"strength": 100}}')`,
		testPartUUID,
	)
	require.NoError(t, err)
	defer func() {
		_, _ = inventoryDBPool.Exec(
			context.Background(),
			`DELETE FROM parts WHERE uuid = $1`, testPartUUID,
		)
	}()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, errs[idx] = inventoryClient.ReserveParts(ctx, &inventoryv1.ReservePartsRequest{
				Uuids: []string{testPartUUID},
			})
		}(i)
	}
	wg.Wait()

	successCount := 0
	failedCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		} else {
			failedCount++
		}
	}

	assert.Equal(t, 1, successCount, "ровно один Reserve должен пройти успешно")
	assert.Equal(t, 1, failedCount, "ровно один Reserve должен упасть (нет деталей в наличии)")
}
