package tests

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/student/order/tests/testutil"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/student/shared/pkg/proto/payment/v1"
)

func TestMain(m *testing.M) {
	code := m.Run()
	_ = testutil.StopShared(context.Background())
	os.Exit(code)
}

// Тесты InventoryService (gRPC)

func TestInventory_GetPart_Success(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: testutil.HullAluminumUUID,
	})
	require.NoError(t, err)

	part := resp.GetPart()
	assert.Equal(t, testutil.HullAluminumUUID, part.GetUuid())
	assert.Equal(t, int64(testutil.HullAluminumPrice), part.GetPrice())
	assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	assert.NotEmpty(t, part.GetName())
	assert.NotNil(t, part.GetCreatedAt())
}

func TestInventory_GetPart_AllTypes(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	testCases := []struct {
		name     string
		uuid     string
		price    int64
		partType inventoryv1.PartType
	}{
		{"Hull Aluminum", testutil.HullAluminumUUID, testutil.HullAluminumPrice, inventoryv1.PartType_PART_TYPE_HULL},
		{"Hull Titanium", testutil.HullTitaniumUUID, testutil.HullTitaniumPrice, inventoryv1.PartType_PART_TYPE_HULL},
		{"Engine Ion C", testutil.EngineIonCUUID, testutil.EngineIonCPrice, inventoryv1.PartType_PART_TYPE_ENGINE},
		{"Engine Ion B", testutil.EngineIonBUUID, testutil.EngineIonBPrice, inventoryv1.PartType_PART_TYPE_ENGINE},
		{"Shield Energy", testutil.ShieldEnergyUUID, testutil.ShieldEnergyPrice, inventoryv1.PartType_PART_TYPE_SHIELD},
		{"Weapon Laser", testutil.WeaponLaserUUID, testutil.WeaponLaserPrice, inventoryv1.PartType_PART_TYPE_WEAPON},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.InventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
				Uuid: tc.uuid,
			})
			require.NoError(t, err)

			part := resp.GetPart()
			assert.Equal(t, tc.uuid, part.GetUuid())
			assert.Equal(t, tc.price, part.GetPrice())
			assert.Equal(t, tc.partType, part.GetPartType())
		})
	}
}

func TestInventory_GetPart_NotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, err := env.InventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: uuid.New().String(),
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_GetPart_EmptyUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, err := env.InventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: "",
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_GetPart_InvalidUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, err := env.InventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: "invalid-uuid-format",
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ListParts_All(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_UNSPECIFIED,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)
}

func TestInventory_ListParts_ByType_Hull(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_HULL,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3) // Алюминиевый, Титановый, Плазменный (stock=0)

	for _, part := range resp.GetParts() {
		assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	}
}

func TestInventory_ListParts_ByType_Engine(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_ENGINE,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	for _, part := range resp.GetParts() {
		assert.Equal(t, inventoryv1.PartType_PART_TYPE_ENGINE, part.GetPartType())
	}
}

func TestInventory_ListParts_ByType_Shield(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_SHIELD,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, testutil.ShieldEnergyUUID, resp.GetParts()[0].GetUuid())
}

func TestInventory_ListParts_ByType_Weapon(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_WEAPON,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, testutil.WeaponLaserUUID, resp.GetParts()[0].GetUuid())
}

func TestInventory_ListParts_SortedByName(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
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
	t.Parallel()
	env := testutil.NewEnv(t)

	uuids := []string{testutil.HullAluminumUUID, testutil.EngineIonCUUID, testutil.ShieldEnergyUUID}

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
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
	t.Parallel()
	env := testutil.NewEnv(t)

	// Запрос в определённом порядке: Engine, Hull, Weapon
	uuids := []string{testutil.EngineIonCUUID, testutil.HullAluminumUUID, testutil.WeaponLaserUUID}

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
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
	t.Parallel()
	env := testutil.NewEnv(t)

	// Запрос с uuids И part_type — part_type должен быть проигнорирован
	uuids := []string{testutil.HullAluminumUUID, testutil.EngineIonCUUID}

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids:    uuids,
		PartType: inventoryv1.PartType_PART_TYPE_WEAPON, // Должен быть проигнорирован
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	// Проверяем, что получили Hull и Engine, а не Weapons
	assert.Equal(t, testutil.HullAluminumUUID, resp.GetParts()[0].GetUuid())
	assert.Equal(t, testutil.EngineIonCUUID, resp.GetParts()[1].GetUuid())
}

func TestInventory_ListParts_ByUuids_NotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Включаем один несуществующий UUID
	nonExistentUUID := uuid.New().String()
	uuids := []string{testutil.HullAluminumUUID, nonExistentUUID, testutil.EngineIonCUUID}

	_, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_ListParts_ByUuids_InvalidUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	uuids := []string{testutil.HullAluminumUUID, "invalid-uuid-format"}

	_, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ListParts_ByUuids_SingleUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	uuids := []string{testutil.WeaponLaserUUID}

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, testutil.WeaponLaserUUID, resp.GetParts()[0].GetUuid())
	assert.Equal(t, int64(testutil.WeaponLaserPrice), resp.GetParts()[0].GetPrice())
}

func TestInventory_ListParts_ByUuids_AllParts(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Запрашиваем все 6 деталей по UUID
	uuids := []string{
		testutil.HullAluminumUUID, testutil.HullTitaniumUUID,
		testutil.EngineIonCUUID, testutil.EngineIonBUUID,
		testutil.ShieldEnergyUUID, testutil.WeaponLaserUUID,
	}

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
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
	t.Parallel()
	env := testutil.NewEnv(t)

	// Пустой список UUID — должен вернуть все детали (фильтрация по типу UNSPECIFIED)
	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: []string{},
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)
}

// Тесты PaymentService (gRPC)

func TestPayment_PayOrder_Success_Card(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
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
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_SBP,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_Success_CreditCard(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_Success_InvestorMoney(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_EmptyOrderUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestPayment_PayOrder_UnspecifiedMethod(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestPayment_PayOrder_UniqueTransactions(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	orderUUID := uuid.New().String()

	resp1, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)

	resp2, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetTransactionUuid(), resp2.GetTransactionUuid(),
		"каждый платёж должен генерировать уникальный UUID транзакции")
}

// Тесты OrderService (HTTP)

func TestOrder_Create_Success_MinimalParts(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}

	result, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OrderUUID)
	assert.Equal(t, int64(testutil.HullAluminumPrice+testutil.EngineIonCPrice), result.TotalPrice)
}

func TestOrder_Create_Success_AllParts(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullTitaniumUUID,
		EngineUUID: testutil.EngineIonBUUID,
		ShieldUUID: new(testutil.ShieldEnergyUUID),
		WeaponUUID: new(testutil.WeaponLaserUUID),
	}

	result, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OrderUUID)

	expectedTotal := int64(testutil.HullTitaniumPrice + testutil.EngineIonBPrice + testutil.ShieldEnergyPrice + testutil.WeaponLaserPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

func TestOrder_Create_VerifyTotalPrice(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID, // 500000
		EngineUUID: testutil.EngineIonCUUID,   // 300000
	}

	result, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, int64(800000), result.TotalPrice, "500000 + 300000 = 800000")
}

func TestOrder_Create_HullNotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   uuid.New().String(),
		EngineUUID: testutil.EngineIonCUUID,
	}

	_, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_EngineNotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: uuid.New().String(),
	}

	_, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_ShieldNotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
		ShieldUUID: new(uuid.New().String()),
	}

	_, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_WeaponNotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
		WeaponUUID: new(uuid.New().String()),
	}

	_, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Get_Success(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Сначала создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Получаем заказ
	order, resp := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, order)
	assert.Equal(t, createResult.OrderUUID, order.OrderUUID)
	assert.Equal(t, testutil.HullAluminumUUID, order.HullUUID)
	assert.Equal(t, testutil.EngineIonCUUID, order.EngineUUID)
	assert.Equal(t, createResult.TotalPrice, order.TotalPrice)
}

func TestOrder_Get_VerifyStatus_PendingPayment(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Получаем и проверяем статус
	order, resp := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "PENDING_PAYMENT", order.Status)
}

func TestOrder_Get_NotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, resp := env.GetOrder(t, uuid.New().String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Pay_Success_Card(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ
	payReq := &testutil.PayOrderRequest{PaymentMethod: "CARD"}
	payResult, resp := env.PayOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, payResult)
	assert.NotEmpty(t, payResult.TransactionUUID)
}

func TestOrder_Pay_VerifyStatusChange(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ
	payReq := &testutil.PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := env.PayOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()

	// Получаем и проверяем статус changed to PAID
	order, getResp := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "PAID", order.Status)
	assert.NotNil(t, order.TransactionUUID)
	assert.NotNil(t, order.PaymentMethod)
	assert.Equal(t, "CARD", *order.PaymentMethod)
}

func TestOrder_Pay_NotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	payReq := &testutil.PayOrderRequest{PaymentMethod: "CARD"}
	_, resp := env.PayOrder(t, uuid.New().String(), payReq)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Pay_AlreadyPaid(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ в первый раз
	payReq := &testutil.PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp1 := env.PayOrder(t, createResult.OrderUUID, payReq)
	_ = payResp1.Body.Close()

	// Пытаемся оплатить повторно — должна быть ошибка конфликта
	_, payResp2 := env.PayOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp2.Body.Close() }()

	require.Equal(t, http.StatusConflict, payResp2.StatusCode)
}

func TestOrder_Pay_AlreadyCancelled(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ
	_, cancelResp := env.CancelOrder(t, createResult.OrderUUID)
	_ = cancelResp.Body.Close()

	// Пытаемся оплатить отменённый заказ — должна быть ошибка конфликта
	payReq := &testutil.PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := env.PayOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, payResp.StatusCode)
}

func TestOrder_Cancel_Success(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ
	_, resp := env.CancelOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestOrder_Cancel_VerifyStatusChange(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ
	_, cancelResp := env.CancelOrder(t, createResult.OrderUUID)
	_ = cancelResp.Body.Close()

	// Получаем и проверяем статус changed to CANCELLED
	order, getResp := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "CANCELLED", order.Status)
}

func TestOrder_Cancel_NotFound(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, resp := env.CancelOrder(t, uuid.New().String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Cancel_AlreadyPaid(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Оплачиваем заказ
	payReq := &testutil.PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := env.PayOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()

	// Пытаемся отменить оплаченный заказ — должна быть ошибка конфликта
	_, cancelResp := env.CancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp.StatusCode)
}

func TestOrder_Cancel_AlreadyCancelled(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Отменяем заказ first time
	_, cancelResp1 := env.CancelOrder(t, createResult.OrderUUID)
	_ = cancelResp1.Body.Close()

	// Пытаемся отменить повторно — должна быть ошибка конфликта
	_, cancelResp2 := env.CancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp2.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp2.StatusCode)
}

// Дополнительные тесты валидации

func TestOrder_Create_WithWeaponOnly(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
		WeaponUUID: new(testutil.WeaponLaserUUID),
	}

	result, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	expectedTotal := int64(testutil.HullAluminumPrice + testutil.EngineIonCPrice + testutil.WeaponLaserPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

func TestOrder_Pay_AllMethods(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	methods := []string{"CARD", "SBP", "CREDIT_CARD", "INVESTOR_MONEY"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			// Создаём заказ
			createReq := &testutil.CreateOrderRequest{
				HullUUID:   testutil.HullAluminumUUID,
				EngineUUID: testutil.EngineIonCUUID,
			}
			createResult, createResp := env.CreateOrder(t, createReq)
			_ = createResp.Body.Close()
			require.NotNil(t, createResult)

			// Оплачиваем этим методом
			payReq := &testutil.PayOrderRequest{PaymentMethod: method}
			payResult, resp := env.PayOrder(t, createResult.OrderUUID, payReq)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotNil(t, payResult)
			assert.NotEmpty(t, payResult.TransactionUUID)

			// Проверяем, что метод оплаты сохранён
			order, getResp := env.GetOrder(t, createResult.OrderUUID)
			_ = getResp.Body.Close()
			require.NotNil(t, order.PaymentMethod)
			assert.Equal(t, method, *order.PaymentMethod)
		})
	}
}

func TestOrder_Get_WithOptionalParts(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	shieldUUID := testutil.ShieldEnergyUUID
	weaponUUID := testutil.WeaponLaserUUID
	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
		ShieldUUID: &shieldUUID,
		WeaponUUID: &weaponUUID,
	}

	createResult, createResp := env.CreateOrder(t, req)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Получаем заказ и проверяем, что опциональные детали сохранены
	order, resp := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, order.ShieldUUID)
	require.NotNil(t, order.WeaponUUID)
	assert.Equal(t, shieldUUID, *order.ShieldUUID)
	assert.Equal(t, weaponUUID, *order.WeaponUUID)
}

func TestPayment_PayOrder_InvalidUUIDFormat(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	_, err := env.PaymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "invalid-uuid-format",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

// Тесты полного жизненного цикла

func TestOrder_FullLifecycle_CreatePayGet(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// 1. Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullTitaniumUUID,
		EngineUUID: testutil.EngineIonBUUID,
		ShieldUUID: new(testutil.ShieldEnergyUUID),
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)
	assert.NotEmpty(t, createResult.OrderUUID)

	expectedTotal := int64(testutil.HullTitaniumPrice + testutil.EngineIonBPrice + testutil.ShieldEnergyPrice)
	assert.Equal(t, expectedTotal, createResult.TotalPrice)

	// 2. Получаем заказ — проверяем PENDING_PAYMENT
	order1, getResp1 := env.GetOrder(t, createResult.OrderUUID)
	_ = getResp1.Body.Close()
	assert.Equal(t, "PENDING_PAYMENT", order1.Status)
	assert.Nil(t, order1.TransactionUUID)

	// 3. Оплачиваем заказ
	payReq := &testutil.PayOrderRequest{PaymentMethod: "SBP"}
	payResult, payResp := env.PayOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()
	require.NotNil(t, payResult)
	assert.NotEmpty(t, payResult.TransactionUUID)

	// 4. Получаем заказ — проверяем PAID
	order2, getResp2 := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "PAID", order2.Status)
	require.NotNil(t, order2.TransactionUUID)
	assert.Equal(t, payResult.TransactionUUID, *order2.TransactionUUID)
	require.NotNil(t, order2.PaymentMethod)
	assert.Equal(t, "SBP", *order2.PaymentMethod)
}

func TestOrder_FullLifecycle_CreateCancelGet(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// 1. Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// 2. Получаем заказ — проверяем PENDING_PAYMENT
	order1, getResp1 := env.GetOrder(t, createResult.OrderUUID)
	_ = getResp1.Body.Close()
	assert.Equal(t, "PENDING_PAYMENT", order1.Status)

	// 3. Отменяем заказ
	_, cancelResp := env.CancelOrder(t, createResult.OrderUUID)
	_ = cancelResp.Body.Close()

	// 4. Получаем заказ — проверяем CANCELLED
	order2, getResp2 := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "CANCELLED", order2.Status)
	assert.Nil(t, order2.TransactionUUID)
}

func TestOrder_FullLifecycle_AllPartsPayGet(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Полный жизненный цикл со всеми 4 деталями: hull + engine + shield + weapon
	shieldUUID := testutil.ShieldEnergyUUID
	weaponUUID := testutil.WeaponLaserUUID
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullTitaniumUUID,
		EngineUUID: testutil.EngineIonBUUID,
		ShieldUUID: &shieldUUID,
		WeaponUUID: &weaponUUID,
	}

	// 1. Создаём заказ
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	expectedTotal := int64(testutil.HullTitaniumPrice + testutil.EngineIonBPrice + testutil.ShieldEnergyPrice + testutil.WeaponLaserPrice)
	assert.Equal(t, expectedTotal, createResult.TotalPrice)

	// 2. Проверяем все детали в GET ответе
	order1, getResp1 := env.GetOrder(t, createResult.OrderUUID)
	_ = getResp1.Body.Close()
	assert.Equal(t, testutil.HullTitaniumUUID, order1.HullUUID)
	assert.Equal(t, testutil.EngineIonBUUID, order1.EngineUUID)
	require.NotNil(t, order1.ShieldUUID)
	assert.Equal(t, shieldUUID, *order1.ShieldUUID)
	require.NotNil(t, order1.WeaponUUID)
	assert.Equal(t, weaponUUID, *order1.WeaponUUID)

	// 3. Оплачиваем заказ
	payReq := &testutil.PayOrderRequest{PaymentMethod: "CREDIT_CARD"}
	payResult, payResp := env.PayOrder(t, createResult.OrderUUID, payReq)
	_ = payResp.Body.Close()
	require.NotNil(t, payResult)

	// 4. Проверяем финальное состояние
	order2, getResp2 := env.GetOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "PAID", order2.Status)
	require.NotNil(t, order2.PaymentMethod)
	assert.Equal(t, "CREDIT_CARD", *order2.PaymentMethod)
}

// Тесты ogen-валидации (400 Bad Request)

func TestOrder_Create_InvalidBody_EmptyJSON(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders", bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_NotJSON(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_MissingHullUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	body := `{"engine_uuid": "` + testutil.EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_MissingEngineUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	body := `{"hull_uuid": "` + testutil.HullAluminumUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_InvalidHullUUID(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	body := `{"hull_uuid": "not-a-uuid", "engine_uuid": "` + testutil.EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Get_InvalidUUIDInPath(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.HTTPClient.Get(env.BaseURL + "/api/v1/orders/not-a-uuid")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_InvalidUUIDInPath(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	body := `{"payment_method": "CARD"}`
	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders/not-a-uuid/pay", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_InvalidPaymentMethod(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Пытаемся оплатить невалидным методом — ogen отклонит
	body := `{"payment_method": "BITCOIN"}`
	httpReq, err := http.NewRequest(http.MethodPost,
		env.BaseURL+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_MissingPaymentMethod(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	// Пытаемся оплатить без payment_method
	body := `{}`
	httpReq, err := http.NewRequest(http.MethodPost,
		env.BaseURL+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_EmptyBody(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Создаём заказ
	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	httpReq, err := http.NewRequest(http.MethodPost,
		env.BaseURL+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte("")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Cancel_InvalidUUIDInPath(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	httpReq, err := http.NewRequest(http.MethodPost, env.BaseURL+"/api/v1/orders/not-a-uuid/cancel", nil)
	require.NoError(t, err)

	resp, err := env.HTTPClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Тесты out of stock

func TestOrder_Create_OutOfStock_Hull(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Плазменный корпус — stock_quantity=0, заказ должен быть отклонён
	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullOutOfStockUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}

	_, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestOrder_Create_OutOfStock_WithOptionalParts(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	// Out of stock деталь среди опциональных — shield
	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
		ShieldUUID: new(testutil.HullOutOfStockUUID), // Передаём hull UUID как shield — тип не совпадёт, но out of stock проверяется первым
	}

	_, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	// Либо Conflict (out of stock), либо другая ошибка — не 201.
	assert.NotEqual(t, http.StatusCreated, resp.StatusCode)
}

// Тесты Inventory: out of stock деталь присутствует в списке

func TestInventory_GetPart_OutOfStock(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	resp, err := env.InventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: testutil.HullOutOfStockUUID,
	})
	require.NoError(t, err)

	part := resp.GetPart()
	assert.Equal(t, testutil.HullOutOfStockUUID, part.GetUuid())
	assert.Equal(t, int64(testutil.HullOutOfStockPrice), part.GetPrice())
	assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	assert.Equal(t, int64(0), part.GetStockQuantity())
}

func TestInventory_ListParts_ByUuids_IncludesOutOfStock(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	uuids := []string{testutil.HullAluminumUUID, testutil.HullOutOfStockUUID}

	resp, err := env.InventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	// Out of stock деталь возвращается — фильтрации по наличию нет
	assert.Equal(t, testutil.HullOutOfStockUUID, resp.GetParts()[1].GetUuid())
	assert.Equal(t, int64(0), resp.GetParts()[1].GetStockQuantity())
}

// Тесты Order: проверка created_at

func TestOrder_Get_VerifyCreatedAt(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

	createReq := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
	}
	createResult, createResp := env.CreateOrder(t, createReq)
	_ = createResp.Body.Close()
	require.NotNil(t, createResult)

	order, resp := env.GetOrder(t, createResult.OrderUUID)
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
	t.Parallel()
	env := testutil.NewEnv(t)

	req := &testutil.CreateOrderRequest{
		HullUUID:   testutil.HullAluminumUUID,
		EngineUUID: testutil.EngineIonCUUID,
		ShieldUUID: new(testutil.ShieldEnergyUUID),
	}

	result, resp := env.CreateOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	expectedTotal := int64(testutil.HullAluminumPrice + testutil.EngineIonCPrice + testutil.ShieldEnergyPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}
