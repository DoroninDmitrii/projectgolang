package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertOrderStatus проверяет статус заказа в БД напрямую.
// Используется как контрольная точка после API-вызовов: убеждаемся,
// что состояние не только в ответе, но и записано в хранилище.
func (e *Env) AssertOrderStatus(t *testing.T, orderUUID, want string) {
	t.Helper()

	var got string
	err := e.OrderPool.QueryRow(context.Background(),
		`SELECT status FROM orders WHERE uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err, "не удалось прочитать заказ %s", orderUUID)
	assert.Equal(t, want, got, "статус заказа в БД")
}

// AssertOrderTransaction проверяет, что в БД заказа есть transaction_uuid и payment_method.
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

// PartReserved возвращает текущее значение reserved для детали.
func (e *Env) PartReserved(t *testing.T, partUUID string) int {
	t.Helper()

	var got int
	err := e.InventoryPool.QueryRow(context.Background(),
		`SELECT reserved FROM parts WHERE uuid = $1`, partUUID).Scan(&got)
	require.NoError(t, err)
	return got
}

// AssertPartReserved сравнивает резерв с ожидаемым значением.
func (e *Env) AssertPartReserved(t *testing.T, partUUID string, want int) {
	t.Helper()
	assert.Equal(t, want, e.PartReserved(t, partUUID),
		"reserved для детали %s", partUUID)
}
