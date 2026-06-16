//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
)

// HTTP DTOs дублируются с api_test намеренно: e2e — самостоятельная сьюта,
// которая держит свой контракт с HTTP-API явно перед глазами

type createOrderRequest struct {
	UserUUID   string  `json:"user_uuid"`
	HullUUID   string  `json:"hull_uuid"`
	EngineUUID string  `json:"engine_uuid"`
	ShieldUUID *string `json:"shield_uuid,omitempty"`
	WeaponUUID *string `json:"weapon_uuid,omitempty"`
}

type createOrderResponse struct {
	OrderUUID  string `json:"order_uuid"`
	TotalPrice int64  `json:"total_price"`
}

type payOrderRequest struct {
	PaymentMethod string `json:"payment_method"`
}

type payOrderResponse struct {
	TransactionUUID string `json:"transaction_uuid"`
}

type orderDTO struct {
	OrderUUID       string  `json:"order_uuid"`
	UserUUID        string  `json:"user_uuid"`
	HullUUID        string  `json:"hull_uuid"`
	EngineUUID      string  `json:"engine_uuid"`
	TotalPrice      int64   `json:"total_price"`
	TransactionUUID *string `json:"transaction_uuid"`
	PaymentMethod   *string `json:"payment_method"`
	Status          string  `json:"status"`
}

// TestE2E_OrderFullLifecycle_Assembled — happy-path через ВСЮ Kafka-цепочку.
//
// Шаги:
//  1. POST /orders — создаём заказ, ждём 201
//  2. POST /orders/{uuid}/pay — оплачиваем, ждём 200 и статус PAID
//  3. order продьюсит OrderPaid → test-assembler → ShipAssembled
//  4. order ship-assembled-консьюмер обрабатывает событие, переводит в ASSEMBLED
//  5. Eventually GET /orders/{uuid} — статус ASSEMBLED, transaction_uuid сохранён
//  6. Проверяем, что CommitParts действительно списал stock_quantity
func TestE2E_OrderFullLifecycle_Assembled(t *testing.T) {
	ctx := context.Background()

	// Снимок stock_quantity ДО заказа — для проверки CommitParts позже.
	// Используем общий пул деталей (HullAluminum + EngineIonC), значит другие
	// e2e-тесты могли бы интерферировать. Сейчас тест в сьюте один — ок,
	// если будет больше, лучше брать уникальные seed-детали под каждый
	stockBefore := getStock(ctx, t, []string{HullAluminumUUID, EngineIonCUUID})

	// 1. Create
	order := mustCreateOrder(t, &createOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	require.Equal(t, int64(HullAluminumPrice+EngineIonCPrice), order.TotalPrice)

	// Сразу после Create — статус PENDING_PAYMENT
	got := mustGetOrder(t, order.OrderUUID)
	require.Equal(t, "PENDING_PAYMENT", got.Status)
	require.Nil(t, got.TransactionUUID)

	// 2. Pay
	pay := mustPayOrder(t, order.OrderUUID, &payOrderRequest{PaymentMethod: "CARD"})
	require.NotEmpty(t, pay.TransactionUUID)

	// Сразу после Pay — статус PAID. ASSEMBLED ещё не наступил, цепочка асинхронная
	got = mustGetOrder(t, order.OrderUUID)
	require.Equal(t, "PAID", got.Status)
	require.NotNil(t, got.TransactionUUID)
	assert.Equal(t, pay.TransactionUUID, *got.TransactionUUID)

	// 3-5. Ждём ASSEMBLED. Таймаут с запасом: Sarama-консьюмеру нужно
	// зарегистрироваться в группе (может занять несколько секунд при первом
	// rebalance), плюс round-trip сообщений через Redpanda
	waitForOrderStatus(t, order.OrderUUID, "ASSEMBLED", 30*time.Second)

	// Финальная проверка: все ключевые поля сохранены, цепочка прошла полностью
	final := mustGetOrder(t, order.OrderUUID)
	assert.Equal(t, "ASSEMBLED", final.Status)
	require.NotNil(t, final.TransactionUUID)
	assert.Equal(t, pay.TransactionUUID, *final.TransactionUUID)
	require.NotNil(t, final.PaymentMethod)
	assert.Equal(t, "CARD", *final.PaymentMethod)

	// 6. CommitParts должен был списать по 1 от каждой использованной детали.
	// Это контракт ShipAssembledHandler → InventoryClient.CommitParts —
	// именно его проверка, которой не хватает в api_test (там noopProducer)
	stockAfter := getStock(ctx, t, []string{HullAluminumUUID, EngineIonCUUID})
	assert.Equal(t, stockBefore[HullAluminumUUID]-1, stockAfter[HullAluminumUUID],
		"hull stock должен уменьшиться на 1 после ASSEMBLED")
	assert.Equal(t, stockBefore[EngineIonCUUID]-1, stockAfter[EngineIonCUUID],
		"engine stock должен уменьшиться на 1 после ASSEMBLED")
}

// =============================================================================
// HTTP helpers
// =============================================================================

func mustCreateOrder(t *testing.T, req *createOrderRequest) *createOrderResponse {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/orders", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out createOrderResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return &out
}

func mustPayOrder(t *testing.T, orderUUID string, req *payOrderRequest) *payOrderResponse {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/orders/"+orderUUID+"/pay", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out payOrderResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return &out
}

func mustGetOrder(t *testing.T, orderUUID string) *orderDTO {
	t.Helper()

	resp, err := httpClient.Get(ts.URL + "/api/v1/orders/" + orderUUID)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out orderDTO
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return &out
}

// waitForOrderStatus поллит GET /orders/{uuid}, пока статус не совпадёт с expected.
// Используем require.Eventually — это идиоматический паттерн ожидания в Go-тестах:
// явно описывает «жду конечного состояния», а не «жду фиксированный интервал».
// 200мс между попытками — компромисс: чаще = лишняя нагрузка, реже = пропустим
// окно между сменой статуса и тестовой паузой.
func waitForOrderStatus(t *testing.T, orderUUID, expected string, timeout time.Duration) {
	t.Helper()

	var lastStatus string
	require.Eventuallyf(t, func() bool {
		got := mustGetOrder(t, orderUUID)
		lastStatus = got.Status
		return got.Status == expected
	}, timeout, 200*time.Millisecond,
		"order did not reach expected status: order_uuid=%s expected=%s last_seen=%s",
		orderUUID, expected, lastStatus)
}

// =============================================================================
// Inventory helper
// =============================================================================

func getStock(ctx context.Context, t *testing.T, partUUIDs []string) map[string]int64 {
	t.Helper()

	stocks := make(map[string]int64, len(partUUIDs))
	for _, u := range partUUIDs {
		resp, err := inventoryClient.GetPart(ctx, &inventoryv1.GetPartRequest{Uuid: u})
		require.NoError(t, err, "GetPart %s", u)
		stocks[u] = resp.GetPart().GetStockQuantity()
	}
	return stocks
}
