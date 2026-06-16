package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateOrderRequest — тело запроса POST /api/v1/orders.
type CreateOrderRequest struct {
	HullUUID   string  `json:"hull_uuid"`
	EngineUUID string  `json:"engine_uuid"`
	ShieldUUID *string `json:"shield_uuid,omitempty"`
	WeaponUUID *string `json:"weapon_uuid,omitempty"`
}

// CreateOrderResponse — ответ на создание заказа.
type CreateOrderResponse struct {
	OrderUUID  string `json:"order_uuid"`
	TotalPrice int64  `json:"total_price"`
}

// PayOrderRequest — тело запроса POST /api/v1/orders/{uuid}/pay.
type PayOrderRequest struct {
	PaymentMethod string `json:"payment_method"`
}

// PayOrderResponse — ответ на оплату заказа.
type PayOrderResponse struct {
	TransactionUUID string `json:"transaction_uuid"`
}

// CancelOrderResponse — ответ на отмену заказа (пустой).
type CancelOrderResponse struct{}

// OrderDTO — представление заказа в ответе API.
type OrderDTO struct {
	OrderUUID       string  `json:"order_uuid"`
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

// CreateOrder делает POST /api/v1/orders. При StatusCreated декодирует ответ.
func (e *Env) CreateOrder(t *testing.T, req *CreateOrderRequest) (*CreateOrderResponse, *http.Response) {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, e.BaseURL+"/api/v1/orders", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.HTTPClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusCreated {
		var result CreateOrderResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return &result, resp
	}
	return nil, resp
}

// GetOrder делает GET /api/v1/orders/{uuid}.
func (e *Env) GetOrder(t *testing.T, orderUUID string) (*OrderDTO, *http.Response) {
	t.Helper()

	resp, err := e.HTTPClient.Get(e.BaseURL + "/api/v1/orders/" + orderUUID)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result OrderDTO
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return &result, resp
	}
	return nil, resp
}

// PayOrder делает POST /api/v1/orders/{uuid}/pay.
func (e *Env) PayOrder(t *testing.T, orderUUID string, req *PayOrderRequest) (*PayOrderResponse, *http.Response) {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost,
		e.BaseURL+"/api/v1/orders/"+orderUUID+"/pay", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.HTTPClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result PayOrderResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return &result, resp
	}
	return nil, resp
}

// CancelOrder делает POST /api/v1/orders/{uuid}/cancel.
func (e *Env) CancelOrder(t *testing.T, orderUUID string) (*CancelOrderResponse, *http.Response) {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodPost,
		e.BaseURL+"/api/v1/orders/"+orderUUID+"/cancel", nil)
	require.NoError(t, err)

	resp, err := e.HTTPClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result CancelOrderResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return &result, resp
	}
	return nil, resp
}
