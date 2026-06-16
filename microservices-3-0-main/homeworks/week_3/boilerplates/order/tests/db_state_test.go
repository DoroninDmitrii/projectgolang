package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/student/order/tests/testutil"
)

// Тесты этого файла дополняют api_test.go: после публичных API-вызовов
// проверяют состояние в БД напрямую — это ловит баги, при которых API
// возвращает корректный ответ, но запись не доехала до хранилища.

func TestDB_Order_Create_PersistsStatusAndItems(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	created, resp := env.CreateOrder(t, &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullTitaniumUUID,
		EngineUUID: testutil.EngineIonBUUID,
		ShieldUUID: new(testutil.ShieldEnergyUUID),
		WeaponUUID: new(testutil.WeaponLaserUUID),
	})
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

	payResult, payResp := env.PayOrder(t, created.OrderUUID, &testutil.PayOrderRequest{PaymentMethod: "SBP"})
	_ = payResp.Body.Close()
	require.Equal(t, http.StatusOK, payResp.StatusCode)
	require.NotNil(t, payResult)

	env.AssertOrderStatus(t, created.OrderUUID, "PAID")
	env.AssertOrderTransactionEquals(t, created.OrderUUID, payResult.TransactionUUID, "SBP")
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
	env.AssertOrderStatus(t, created.OrderUUID, "PAID")
}
