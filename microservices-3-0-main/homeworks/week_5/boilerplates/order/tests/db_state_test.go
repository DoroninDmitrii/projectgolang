package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/student/order/tests/testutil"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
)

// Тесты этого файла дополняют api_test.go: после публичных API-вызовов
// они проверяют состояние в БД напрямую — это ловит баги, при которых
// API возвращает корректный ответ, но запись не доехала до хранилища.

// assertOrderStatusInDB читает status напрямую из orders.
func assertOrderStatusInDB(t *testing.T, orderUUID, expected string) {
	t.Helper()
	var got string
	err := orderDBPool.QueryRow(context.Background(),
		`SELECT status FROM orders WHERE uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, expected, got, "status в orders должен быть %s", expected)
}

// assertOrderItemsCount проверяет количество позиций в order_items.
func assertOrderItemsCount(t *testing.T, orderUUID string, expected int) {
	t.Helper()
	var count int
	err := orderDBPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM order_items WHERE order_uuid = $1`, orderUUID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, expected, count, "в order_items должно быть %d позиций", expected)
}

// assertOrderTransaction проверяет, что transaction_uuid и payment_method
// записались в БД после Pay.
func assertOrderTransaction(t *testing.T, orderUUID, expectedMethod string) {
	t.Helper()
	var (
		txUUID *string
		method *string
	)
	err := orderDBPool.QueryRow(context.Background(),
		`SELECT transaction_uuid::text, payment_method FROM orders WHERE uuid = $1`, orderUUID).
		Scan(&txUUID, &method)
	require.NoError(t, err)
	require.NotNil(t, txUUID, "transaction_uuid должен быть заполнен после Pay")
	assert.NotEmpty(t, *txUUID)
	require.NotNil(t, method)
	assert.Equal(t, expectedMethod, *method)
}

// assertOrderUserUUID проверяет, что в БД сохранён переданный user_uuid.
func assertOrderUserUUID(t *testing.T, orderUUID, expected string) {
	t.Helper()
	var got string
	err := orderDBPool.QueryRow(context.Background(),
		`SELECT user_uuid::text FROM orders WHERE uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

// partReserved читает значение reserved для детали из inventory.
func partReserved(t *testing.T, partUUID string) int {
	t.Helper()
	var v int
	err := inventoryDBPool.QueryRow(context.Background(),
		`SELECT reserved FROM parts WHERE uuid = $1`, partUUID).Scan(&v)
	require.NoError(t, err)
	return v
}

// partStock читает значение stock_quantity для детали из inventory.
func partStock(t *testing.T, partUUID string) int {
	t.Helper()
	var v int
	err := inventoryDBPool.QueryRow(context.Background(),
		`SELECT stock_quantity FROM parts WHERE uuid = $1`, partUUID).Scan(&v)
	require.NoError(t, err)
	return v
}

func TestDB_Order_Create_PersistsStatusAndUserUUID(t *testing.T) {
	userUUID := uuid.New().String()
	created, resp := createOrder(t, &CreateOrderRequest{
		UserUUID:   userUUID,
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: new(ShieldEnergyUUID),
		WeaponUUID: new(WeaponLaserUUID),
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, created)

	assertOrderStatusInDB(t, created.OrderUUID, "PENDING_PAYMENT")
	assertOrderItemsCount(t, created.OrderUUID, 4)
	assertOrderUserUUID(t, created.OrderUUID, userUUID)
	testutil.AssertOrderItemsTotalPrice(t, orderDBPool, created.OrderUUID, created.TotalPrice)
}

func TestDB_Order_Pay_PersistsTransactionAndStatus(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, payResp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "SBP"})
	_ = payResp.Body.Close()
	require.Equal(t, http.StatusOK, payResp.StatusCode)

	assertOrderStatusInDB(t, created.OrderUUID, "PAID")
	assertOrderTransaction(t, created.OrderUUID, "SBP")
}

func TestDB_Order_Cancel_PersistsStatus(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, cancelResp := cancelOrder(t, created.OrderUUID)
	_ = cancelResp.Body.Close()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	assertOrderStatusInDB(t, created.OrderUUID, "CANCELLED")
}

// TestDB_Order_Create_IncrementsReserved проверяет, что после успешного Create
// в parts.reserved действительно увеличилось значение для каждой детали.
// Используем уникальную деталь — иначе параллельные тесты конкурировали бы за
// reserved в seed-данных.
func TestDB_Order_Create_IncrementsReserved(t *testing.T) {
	hullUUID := uuid.New().String()
	engineUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES
		   ($1, 'DB-state hull', '', 'HULL', 1000, 5, '{"hull": {"strength": 100}}'),
		   ($2, 'DB-state engine', '', 'ENGINE', 1000, 5, '{"engine": {"class": "C", "required_strength": 50}}')`,
		hullUUID, engineUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid IN ($1, $2)`, hullUUID, engineUUID)
	})

	created, resp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   hullUUID,
		EngineUUID: engineUUID,
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, created)

	assert.Equal(t, 1, partReserved(t, hullUUID), "reserved у hull должен стать 1")
	assert.Equal(t, 1, partReserved(t, engineUUID), "reserved у engine должен стать 1")
}

// TestDB_Order_FailedCreate_DoesNotLeakReserved: при ошибке Create
// (out-of-stock корпус) резерв engine не должен зафиксироваться —
// транзакция должна откатиться целиком.
func TestDB_Order_FailedCreate_DoesNotLeakReserved(t *testing.T) {
	engineUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES ($1, 'DB-state engine leak', '', 'ENGINE', 1000, 5, '{"engine": {"class": "C", "required_strength": 50}}')`,
		engineUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid = $1`, engineUUID)
	})

	engineBefore := partReserved(t, engineUUID)

	_, resp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullOutOfStockUUID,
		EngineUUID: engineUUID,
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	assert.Equal(t, engineBefore, partReserved(t, engineUUID),
		"reserved у engine не должен расти при провале создания заказа")
}

// TestDB_Order_Cancel_DecrementsReserved проверяет, что после Cancel
// reserved уменьшился — это критично для week_5 (проверка Release).
func TestDB_Order_Cancel_DecrementsReserved(t *testing.T) {
	hullUUID := uuid.New().String()
	engineUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES
		   ($1, 'DB-state cancel hull', '', 'HULL', 1000, 5, '{"hull": {"strength": 100}}'),
		   ($2, 'DB-state cancel engine', '', 'ENGINE', 1000, 5, '{"engine": {"class": "C", "required_strength": 50}}')`,
		hullUUID, engineUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid IN ($1, $2)`, hullUUID, engineUUID)
	})

	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   hullUUID,
		EngineUUID: engineUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)
	require.Equal(t, 1, partReserved(t, hullUUID))

	_, cancelResp := cancelOrder(t, created.OrderUUID)
	_ = cancelResp.Body.Close()

	assert.Equal(t, 0, partReserved(t, hullUUID), "reserved у hull должен вернуться в 0 после Cancel")
	assert.Equal(t, 0, partReserved(t, engineUUID), "reserved у engine должен вернуться в 0 после Cancel")
}

// TestDB_FullLifecycle сравнивает полный путь Create → Pay → Cancel(должен упасть)
// через состояние в БД, а не только через API-ответы.
func TestDB_FullLifecycle(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)
	assertOrderStatusInDB(t, created.OrderUUID, "PENDING_PAYMENT")

	_, payResp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	_ = payResp.Body.Close()
	assertOrderStatusInDB(t, created.OrderUUID, "PAID")
	assertOrderTransaction(t, created.OrderUUID, "CARD")

	_, cancelResp := cancelOrder(t, created.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()
	require.Equal(t, http.StatusConflict, cancelResp.StatusCode)
	// Status в БД не должен был измениться — заказ остался PAID.
	assertOrderStatusInDB(t, created.OrderUUID, "PAID")
}

// TestDB_Order_AssembledStatus_BlocksCancel — week_5 ввёл статус ASSEMBLED.
// Через API-цепочку добраться до ASSEMBLED без Kafka сложно (это делает только
// AssemblyService consumer'а). В API-тестах на week_5 нет реальной Kafka,
// поэтому ставим статус напрямую в БД и проверяем, что Cancel корректно его
// блокирует. Это эталонный пример «прямая мутация БД для проверки границы
// состояний».
func TestDB_Order_AssembledStatus_BlocksCancel(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	// Прямая мутация БД, чтобы поставить статус ASSEMBLED — это эталонный
	// тестовый приём недели 5: API-флоу ASSEMBLED идёт через Kafka, в API-сьюте
	// её нет, поэтому имитируем результат напрямую.
	_, err := orderDBPool.Exec(context.Background(),
		`UPDATE orders SET status = 'ASSEMBLED' WHERE uuid = $1`, created.OrderUUID)
	require.NoError(t, err)

	_, cancelResp := cancelOrder(t, created.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, cancelResp.StatusCode,
		"Cancel у ASSEMBLED-заказа должен возвращать 409")
	assertOrderStatusInDB(t, created.OrderUUID, "ASSEMBLED")
}

// TestDB_Inventory_Stock_UnchangedByReserve проверяет, что Reserve меняет только
// reserved, а не stock_quantity (уменьшение stock — это уже CommitParts).
func TestDB_Inventory_Stock_UnchangedByReserve(t *testing.T) {
	partUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES ($1, 'DB-state reserve', '', 'WEAPON', 1000, 3, '{"weapon": {"weapon_type": "laser"}}')`,
		partUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid = $1`, partUUID)
	})

	stockBefore := partStock(t, partUUID)
	require.Equal(t, 3, stockBefore)

	_, err = inventoryClient.ReserveParts(context.Background(),
		&inventoryv1.ReservePartsRequest{Uuids: []string{partUUID}})
	require.NoError(t, err)

	assert.Equal(t, stockBefore, partStock(t, partUUID),
		"Reserve не должен трогать stock_quantity, только reserved")
	assert.Equal(t, 1, partReserved(t, partUUID),
		"Reserve должен увеличить reserved на 1")
}
