package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertOrderItemsTotalPrice проверяет, что SUM(price) по строкам заказа в БД
// равна ожидаемой. Зеркалит week_3/week_4 helper'ы — поднимает baseline недели 5
// до того же уровня уверенности: «total в API-ответе действительно равен сумме
// цен сохранённых items», а не пересчитан где-то по дороге.
func AssertOrderItemsTotalPrice(t *testing.T, pool *pgxpool.Pool, orderUUID string, want int64) {
	t.Helper()

	var got int64
	err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(price), 0) FROM order_items WHERE order_uuid = $1`, orderUUID).Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, want, got, "сумма цен строк заказа в БД")
}
