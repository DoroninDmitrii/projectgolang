package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertOrderStatus проверяет статус заказа в БД напрямую.
func (e *Env) AssertOrderStatus(t *testing.T, orderUUID, want string) {
	t.Helper()

	var got string
	err := e.OrderPool.QueryRow(context.Background(),
		`SELECT status FROM orders WHERE uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err, "не удалось прочитать заказ %s", orderUUID)
	assert.Equal(t, want, got, "статус заказа в БД")
}

// AssertOrderTransaction проверяет, что в БД заказа есть transaction_uuid и payment_method.
// Значение transaction_uuid сверяется только на NotEmpty (используется там, где UUID
// генерируется сервисом и в тесте не нужен его конкретный матч).
func (e *Env) AssertOrderTransaction(t *testing.T, orderUUID, wantMethod string) {
	t.Helper()

	var (
		txUUID *string
		method *string
	)
	err := e.OrderPool.QueryRow(context.Background(),
		`SELECT transaction_uuid, payment_method FROM orders WHERE uuid = $1`, orderUUID).
		Scan(&txUUID, &method)
	require.NoError(t, err)

	require.NotNil(t, txUUID, "transaction_uuid должен быть заполнен в БД")
	require.NotNil(t, method, "payment_method должен быть заполнен в БД")
	assert.NotEmpty(t, *txUUID)
	assert.Equal(t, wantMethod, *method)
}

// AssertOrderTransactionEquals проверяет, что transaction_uuid и payment_method в БД
// равны конкретным ожидаемым значениям (например, тем, что вернул API в ответе на Pay).
// Это сильнее, чем AssertOrderTransaction (там transaction_uuid проверяется только на NotEmpty),
// и ловит баги типа «в БД сохранился UUID, но не тот, что отдан клиенту».
func (e *Env) AssertOrderTransactionEquals(t *testing.T, orderUUID, wantTxUUID, wantMethod string) {
	t.Helper()

	var (
		txUUID *string
		method *string
	)
	err := e.OrderPool.QueryRow(context.Background(),
		`SELECT transaction_uuid, payment_method FROM orders WHERE uuid = $1`, orderUUID).
		Scan(&txUUID, &method)
	require.NoError(t, err)

	require.NotNil(t, txUUID, "transaction_uuid должен быть заполнен в БД")
	require.NotNil(t, method, "payment_method должен быть заполнен в БД")
	assert.Equal(t, wantTxUUID, *txUUID, "transaction_uuid в БД должен совпадать с ответом API")
	assert.Equal(t, wantMethod, *method)
}

// AssertOrderItemsCount проверяет количество строк в order_items для заказа.
func (e *Env) AssertOrderItemsCount(t *testing.T, orderUUID string, want int) {
	t.Helper()

	var got int
	err := e.OrderPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM order_items WHERE order_uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, want, got, "количество позиций заказа")
}

// AssertOrderItemsTotalPrice проверяет, что SUM(price) по строкам заказа равна ожидаемой.
func (e *Env) AssertOrderItemsTotalPrice(t *testing.T, orderUUID string, want int64) {
	t.Helper()

	var got int64
	err := e.OrderPool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(price), 0) FROM order_items WHERE order_uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, want, got, "сумма цен строк заказа в БД")
}
