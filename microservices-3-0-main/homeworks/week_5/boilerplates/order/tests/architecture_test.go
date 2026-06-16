package tests

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Архитектурные тесты подчёркивают свойства всей системы, не отдельных
// эндпоинтов. На week_5 они проверяют:
//   1. Бизнес-вычисления (TotalPrice) делает service-слой Order, и они
//      одинаковы для любых комбинаций деталей.
//   2. Доменные ошибки (PartNotFound, AlreadyPaid, AlreadyCancelled, OutOfStock)
//      корректно поднимаются через слои до HTTP-кода.
//   3. Граничная валидация ogen (UUID в path, enum) не доходит до service-слоя.

// TestArch_TotalPrice_ComputedInService: цена клиенту = сумма цен заказанных
// деталей независимо от комбинации. Эта инвариант теперь проверяется на
// service-уровне OrderService, не в HTTP-хендлере.
func TestArch_TotalPrice_ComputedInService(t *testing.T) {
	cases := []struct {
		name string
		req  *CreateOrderRequest
		want int64
	}{
		{
			name: "только корпус и двигатель",
			req: &CreateOrderRequest{
				UserUUID:   uuid.New().String(),
				HullUUID:   HullAluminumUUID,
				EngineUUID: EngineIonCUUID,
			},
			want: HullAluminumPrice + EngineIonCPrice,
		},
		{
			name: "корпус, двигатель и щит",
			req: &CreateOrderRequest{
				UserUUID:   uuid.New().String(),
				HullUUID:   HullTitaniumUUID,
				EngineUUID: EngineIonBUUID,
				ShieldUUID: new(ShieldEnergyUUID),
			},
			want: HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice,
		},
		{
			name: "все четыре слота",
			req: &CreateOrderRequest{
				UserUUID:   uuid.New().String(),
				HullUUID:   HullTitaniumUUID,
				EngineUUID: EngineIonBUUID,
				ShieldUUID: new(ShieldEnergyUUID),
				WeaponUUID: new(WeaponLaserUUID),
			},
			want: HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice + WeaponLaserPrice,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, resp := createOrder(t, tc.req)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusCreated, resp.StatusCode)
			require.NotNil(t, result)
			assert.Equal(t, tc.want, result.TotalPrice,
				"total price считает service-слой как сумму прайсов; HTTP только пробрасывает результат")
		})
	}
}

// Доменные ошибки разных слоёв (client/service/repository) корректно мапятся
// в HTTP-коды через interceptor — это центральное свойство layered-архитектуры.

func TestArch_DomainError_HullNotFound_Returns404(t *testing.T) {
	_, resp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   uuid.New().String(),
		EngineUUID: EngineIonCUUID,
	})
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"PartNotFound из inventory-клиента должен подняться через service до 404")
}

func TestArch_DomainError_PayNonexistentOrder_Returns404(t *testing.T) {
	_, resp := payOrder(t, uuid.New().String(), &PayOrderRequest{PaymentMethod: "CARD"})
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"OrderNotFound из repository должен подняться через service до 404")
}

func TestArch_DomainError_PayPaid_Returns409(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, payResp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	_ = payResp.Body.Close()

	_, resp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"проверку статуса перед оплатой делает service-слой, не HTTP-хендлер")
}

func TestArch_DomainError_CancelPaid_Returns409(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, payResp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	_ = payResp.Body.Close()

	_, resp := cancelOrder(t, created.OrderUUID)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"проверку статуса перед отменой делает service-слой, не HTTP-хендлер")
}

func TestArch_DomainError_OutOfStock_Returns409(t *testing.T) {
	_, resp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullOutOfStockUUID,
		EngineUUID: EngineIonCUUID,
	})
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"OutOfStock из inventory-клиента должен подняться через service до 409")
}

func TestArch_DomainError_CancelCancelled_Returns409(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	_, firstResp := cancelOrder(t, created.OrderUUID)
	_ = firstResp.Body.Close()

	_, resp := cancelOrder(t, created.OrderUUID)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"повторная отмена уже отменённого заказа должна давать 409 (ErrOrderCancelled)")
}

func TestArch_DomainError_InvalidPaymentMethod_Returns400(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	body := []byte(`{"payment_method": "BITCOIN"}`)
	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+created.OrderUUID+"/pay",
		bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"валидация enum payment_method живёт на границе API (ogen), не в service-слое")
}

func TestArch_DomainError_InvalidUUIDInPath_Returns400(t *testing.T) {
	resp, err := httpClient.Get(orderBaseURL() + "/api/v1/orders/not-a-uuid")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"валидация формата UUID живёт на границе API (ogen), не в service-слое")
}
