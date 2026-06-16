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

	"github.com/student/platform/pkg/auth"
	authv1 "github.com/student/shared/pkg/proto/auth/v1"
	commonv1 "github.com/student/shared/pkg/proto/common/v1"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
	userv1 "github.com/student/shared/pkg/proto/user/v1"
)

// HTTP DTOs дублируются с api_test намеренно: e2e — самостоятельная сьюта,
// которая держит свой контракт с HTTP-API явно перед глазами.
// На неделе 6 user_uuid в теле запроса больше не передаётся: OrderService
// берёт его из сессии (middleware кладёт в ctx)

type createOrderRequest struct {
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
//  1. Регистрируемся и логинимся в IAM — получаем sessionUUID для Bearer
//  2. POST /orders — создаём заказ (с Authorization: Bearer), ждём 201
//  3. POST /orders/{uuid}/pay — оплачиваем, ждём 200 и статус PAID
//  4. order продьюсит OrderPaid → реальный AssemblyService → ShipAssembled
//  5. order ship-assembled-консьюмер обрабатывает событие, переводит в ASSEMBLED
//  6. Eventually GET /orders/{uuid} — статус ASSEMBLED, transaction_uuid сохранён
//  7. Проверяем, что CommitParts действительно списал stock_quantity
func TestE2E_OrderFullLifecycle_Assembled(t *testing.T) {
	ctx := context.Background()

	// 1. Регистрация + логин: один пользователь на весь тест
	sessionUUID := registerAndLogin(t, ctx)

	// Снимок stock_quantity ДО заказа — для проверки CommitParts позже.
	// Прокидываем session_uuid через ctx — bufconn-клиент Inventory снабжён
	// SessionForwarder'ом, который положит его в исходящие gRPC metadata
	sessionUUIDParsed, err := uuid.Parse(sessionUUID)
	require.NoError(t, err)
	authCtx := auth.WithSessionUUID(ctx, sessionUUIDParsed)
	stockBefore := getStock(authCtx, t, []string{HullAluminumUUID, EngineIonCUUID})

	// 2. Create
	order := mustCreateOrder(t, sessionUUID, &createOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	require.Equal(t, int64(HullAluminumPrice+EngineIonCPrice), order.TotalPrice)

	// Сразу после Create — статус PENDING_PAYMENT
	got := mustGetOrder(t, sessionUUID, order.OrderUUID)
	require.Equal(t, "PENDING_PAYMENT", got.Status)
	require.Nil(t, got.TransactionUUID)

	// 3. Pay
	pay := mustPayOrder(t, sessionUUID, order.OrderUUID, &payOrderRequest{PaymentMethod: "CARD"})
	require.NotEmpty(t, pay.TransactionUUID)

	// Сразу после Pay — статус PAID. ASSEMBLED ещё не наступил, цепочка асинхронная
	got = mustGetOrder(t, sessionUUID, order.OrderUUID)
	require.Equal(t, "PAID", got.Status)
	require.NotNil(t, got.TransactionUUID)
	assert.Equal(t, pay.TransactionUUID, *got.TransactionUUID)

	// 4-6. Ждём ASSEMBLED. Таймаут с запасом: assembly эмулирует сборку 5-15 сек
	// (захардкожено в week 6), плюс время на rebalance consumer-group и round-trip
	// сообщений через Redpanda. 60 секунд — комфортный запас
	waitForOrderStatus(t, sessionUUID, order.OrderUUID, "ASSEMBLED", 60*time.Second)

	// Финальная проверка: все ключевые поля сохранены, цепочка прошла полностью
	final := mustGetOrder(t, sessionUUID, order.OrderUUID)
	assert.Equal(t, "ASSEMBLED", final.Status)
	require.NotNil(t, final.TransactionUUID)
	assert.Equal(t, pay.TransactionUUID, *final.TransactionUUID)
	require.NotNil(t, final.PaymentMethod)
	assert.Equal(t, "CARD", *final.PaymentMethod)

	// 7. CommitParts должен был списать по 1 от каждой использованной детали.
	// Это контракт ShipAssembledHandler → InventoryClient.CommitParts —
	// именно его проверка, которой не хватает в api_test (там noopProducer)
	stockAfter := getStock(authCtx, t, []string{HullAluminumUUID, EngineIonCUUID})
	assert.Equal(t, stockBefore[HullAluminumUUID]-1, stockAfter[HullAluminumUUID],
		"hull stock должен уменьшиться на 1 после ASSEMBLED")
	assert.Equal(t, stockBefore[EngineIonCUUID]-1, stockAfter[EngineIonCUUID],
		"engine stock должен уменьшиться на 1 после ASSEMBLED")
}

// =============================================================================
// IAM helper
// =============================================================================

// registerAndLogin регистрирует уникального пользователя и сразу логинится.
// Возвращает sessionUUID — его кладём в Authorization: Bearer для всех HTTP-запросов
// и в auth.WithSessionUUID для прямых gRPC-вызовов Inventory
func registerAndLogin(t *testing.T, ctx context.Context) string {
	t.Helper()

	login := "e2e-" + uuid.New().String()[:8]
	const password = "password123"

	_, err := userSvcClient.Register(ctx, &userv1.RegisterRequest{
		Info: &userv1.UserRegistrationInfo{
			Info:     &commonv1.UserInfo{Login: login},
			Password: password,
		},
	})
	require.NoError(t, err, "register")

	loginResp, err := authSvcClient.Login(ctx, &authv1.LoginRequest{
		Login:    login,
		Password: password,
	})
	require.NoError(t, err, "login")

	sessionUUID := loginResp.GetSessionUuid()
	require.NotEmpty(t, sessionUUID, "session uuid")

	return sessionUUID
}

// =============================================================================
// HTTP helpers
// =============================================================================

func mustCreateOrder(t *testing.T, sessionUUID string, req *createOrderRequest) *createOrderResponse {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/orders", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sessionUUID)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out createOrderResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return &out
}

func mustPayOrder(t *testing.T, sessionUUID, orderUUID string, req *payOrderRequest) *payOrderResponse {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/orders/"+orderUUID+"/pay", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sessionUUID)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out payOrderResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return &out
}

func mustGetOrder(t *testing.T, sessionUUID, orderUUID string) *orderDTO {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/orders/"+orderUUID, nil)
	require.NoError(t, err)
	httpReq.Header.Set("Authorization", "Bearer "+sessionUUID)

	resp, err := httpClient.Do(httpReq)
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
func waitForOrderStatus(t *testing.T, sessionUUID, orderUUID, expected string, timeout time.Duration) {
	t.Helper()

	var lastStatus string
	require.Eventuallyf(t, func() bool {
		got := mustGetOrder(t, sessionUUID, orderUUID)
		lastStatus = got.Status
		return got.Status == expected
	}, timeout, 200*time.Millisecond,
		"order did not reach expected status: order_uuid=%s expected=%s last_seen=%s",
		orderUUID, expected, lastStatus)
}

// =============================================================================
// Inventory helper
// =============================================================================

// getStock читает stock_quantity деталей напрямую через bufconn gRPC.
// Вызывающий код должен предварительно положить session_uuid в ctx
// (через auth.WithSessionUUID) — SessionForwarder автоматически
// положит его в outgoing gRPC metadata
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
