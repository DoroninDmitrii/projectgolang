package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/student/order/tests/testutil"
)

// Тесты этого файла дополняют api_test.go: после публичных API-вызовов
// они дополнительно проверяют состояние в БД напрямую — это ловит баги,
// при которых API возвращает корректный ответ, но запись не доехала до хранилища.

func TestDB_Order_Create_PersistsStatusAndItems(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullTitaniumUUID,
		EngineUUID: testutil.EngineIonBUUID,
		ShieldUUID: new(testutil.ShieldEnergyUUID),
		WeaponUUID: new(testutil.WeaponLaserUUID),
	}
	created, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, created)

	env.AssertOrderStatus(t, created.OrderUUID, "PENDING_PAYMENT")
	env.AssertOrderItemsCount(t, created.OrderUUID, 4)
	env.AssertOrderItemsTotalPrice(t, created.OrderUUID, created.TotalPrice)
}

func TestDB_Order_Pay_PersistsTransactionAndStatus(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	created, createResp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, payResp := env.PayOrder(t, created.OrderUUID, &testutil.PayOrderRequest{PaymentMethod: "SBP"})
	_ = payResp.Body.Close()
	require.Equal(t, http.StatusOK, payResp.StatusCode)

	env.AssertOrderStatus(t, created.OrderUUID, "PAID")
	env.AssertOrderTransaction(t, created.OrderUUID, "SBP")
}

func TestDB_Order_Cancel_PersistsStatus(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	created, createResp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, cancelResp := env.CancelOrder(t, created.OrderUUID)
	_ = cancelResp.Body.Close()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	env.AssertOrderStatus(t, created.OrderUUID, "CANCELLED")
}

// TestDB_Order_Create_IncrementsReserved проверяет, что создание заказа реально
// увеличило reserved у деталей в БД inventory (а не только вернуло 201).
func TestDB_Order_Create_IncrementsReserved(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	hullBefore := env.PartReserved(t, testutil.HullAluminumUUID)
	engineBefore := env.PartReserved(t, testutil.EngineIonCUUID)

	created, resp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, created)

	env.AssertPartReserved(t, testutil.HullAluminumUUID, hullBefore+1)
	env.AssertPartReserved(t, testutil.EngineIonCUUID, engineBefore+1)
}

// TestDB_Order_FailedCreate_DoesNotLeakReserved: при ошибке создания
// заказа резерв не должен оставаться (откат транзакции / освобождение).
// Берём out-of-stock корпус — резерв не пройдёт, reserved у engine
// тоже не должен увеличиться.
func TestDB_Order_FailedCreate_DoesNotLeakReserved(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	engineBefore := env.PartReserved(t, testutil.EngineIonCUUID)

	_, resp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullOutOfStockUUID,
		EngineUUID: testutil.EngineIonCUUID,
	})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	// Резерв двигателя не должен был успеть зафиксироваться:
	// либо резерв в одной транзакции с hull, либо released при ошибке.
	assert.Equal(t, engineBefore, env.PartReserved(t, testutil.EngineIonCUUID),
		"reserved для engine не должен расти при провале создания заказа")
}

// TestDB_Order_Cancel_DecrementsReserved проверяет, что Cancel освобождает
// зарезервированные детали в БД inventory (Cancel вызывает ReleaseParts).
// Без этого теста студент может реализовать Cancel как просто смену статуса
// в orders — и api_test.go всё равно пройдёт, потому что внешне ответ корректен.
func TestDB_Order_Cancel_DecrementsReserved(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	hullBefore := env.PartReserved(t, testutil.HullAluminumUUID)
	engineBefore := env.PartReserved(t, testutil.EngineIonCUUID)

	created, createResp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)
	require.Equal(t, hullBefore+1, env.PartReserved(t, testutil.HullAluminumUUID),
		"после Create reserved у hull должен быть +1")

	_, cancelResp := env.CancelOrder(t, created.OrderUUID)
	_ = cancelResp.Body.Close()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	env.AssertPartReserved(t, testutil.HullAluminumUUID, hullBefore)
	env.AssertPartReserved(t, testutil.EngineIonCUUID, engineBefore)
}

// TestDB_FullLifecycle сравнивает полный путь: Create → Pay → Cancel-должен-падать —
// не только через API-ответы, но и через состояние в БД.
func TestDB_FullLifecycle(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	created, createResp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullTitaniumUUID,
		EngineUUID: testutil.EngineIonBUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)
	env.AssertOrderStatus(t, created.OrderUUID, "PENDING_PAYMENT")

	_, payResp := env.PayOrder(t, created.OrderUUID, &testutil.PayOrderRequest{PaymentMethod: "CARD"})
	_ = payResp.Body.Close()
	env.AssertOrderStatus(t, created.OrderUUID, "PAID")
	env.AssertOrderTransaction(t, created.OrderUUID, "CARD")

	_, cancelResp := env.CancelOrder(t, created.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()
	require.Equal(t, http.StatusConflict, cancelResp.StatusCode)
	// Status в БД не должен был измениться — заказ остался PAID.
	env.AssertOrderStatus(t, created.OrderUUID, "PAID")
}
