//go:build apitest

package tests

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
)

// Concurrency-тесты унаследованы из week_4 (где они флапали из-за отсутствия
// SELECT FOR UPDATE) и week_5 (где они были введены вместе с FOR UPDATE).
// Здесь они продолжают работать — пессимистичные блокировки в Inventory
// сохраняются: ListForUpdate берёт row lock на чтении, второй вызов ждёт
// и видит изменённое состояние.
//
// Прямой gRPC-сценарий с ReserveParts покрыт TestInventory_ReserveParts_Concurrent_LastPart
// в api_test.go; тесты ниже добавляют HTTP-цепочку через Order и сценарий
// атомарности транзакции при mixed-stock.

// TestConcurrent_CreateOrder_LastUnit_ExactlyOneSucceeds: два горутина
// одновременно создают заказ на одну и ту же деталь со stock=1.
// FOR UPDATE в Inventory.ReserveParts должен пропустить ровно один —
// второй получит 409 (ErrOutOfStock из inventory маппится в HTTP 409 в
// error_handler.go).
func TestConcurrent_CreateOrder_LastUnit_ExactlyOneSucceeds(t *testing.T) {
	hullUUID := uuid.New().String()
	engineUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES
		   ($1, 'Concurrent last unit hull', '', 'HULL', 1000, 1, '{"hull": {"strength": 100}}'),
		   ($2, 'Concurrent last unit engine', '', 'ENGINE', 1000, 5, '{"engine": {"class": "C", "required_strength": 50}}')`,
		hullUUID, engineUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid IN ($1, $2)`, hullUUID, engineUUID)
	})

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		statuses []int
	)
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			_, resp := createOrder(t, &CreateOrderRequest{
				UserUUID:   uuid.New().String(),
				HullUUID:   hullUUID,
				EngineUUID: engineUUID,
			})
			defer func() { _ = resp.Body.Close() }()
			mu.Lock()
			defer mu.Unlock()
			statuses = append(statuses, resp.StatusCode)
		}()
	}
	wg.Wait()

	created := 0
	conflict := 0
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		}
	}
	assert.Equal(t, 1, created, "должен создаться ровно один заказ (statuses=%v)", statuses)
	assert.Equal(t, 1, conflict, "второй создаваемый заказ должен получить Conflict (statuses=%v)", statuses)
}

// TestConcurrent_Reserve_MixedStock: гонка ReserveParts с батчем из двух
// деталей, где одна доступна, а вторая — out-of-stock (HullOutOfStockUUID,
// stock=0 в seed). Цель — показать целостность транзакции ReserveParts:
// при провале хотя бы одной детали никакие резервы не сохраняются.
func TestConcurrent_Reserve_MixedStock(t *testing.T) {
	availableUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES ($1, 'Concurrent mixed available', '', 'HULL', 1000, 5, '{"hull": {"strength": 100}}')`,
		availableUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid = $1`, availableUUID)
	})

	const workers = 4

	var (
		wg        sync.WaitGroup
		exhausted atomic.Int64
		others    atomic.Int64
	)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(authCtx(context.Background()), 10*time.Second)
			defer cancel()
			_, err := inventoryClient.ReserveParts(ctx,
				&inventoryv1.ReservePartsRequest{
					Uuids: []string{availableUUID, HullOutOfStockUUID},
				})
			require.Error(t, err)
			// Inventory маппит ErrOutOfStock в ResourceExhausted (interceptor/error.go).
			if status.Code(err) == codes.ResourceExhausted {
				exhausted.Add(1)
			} else {
				others.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(0), others.Load(),
		"все вызовы должны падать с ResourceExhausted, других ошибок быть не должно")
	assert.Equal(t, int64(workers), exhausted.Load(),
		"все батчи должны падать целиком из-за HullOutOfStockUUID")

	// Главное: ни одна доступная деталь не должна остаться зарезервированной,
	// потому что транзакция откатилась. Поле reserved нет в proto (это
	// внутренняя детализация Inventory) — читаем напрямую из БД.
	var reserved int
	err = inventoryDBPool.QueryRow(context.Background(),
		`SELECT reserved FROM parts WHERE uuid = $1`, availableUUID).Scan(&reserved)
	require.NoError(t, err)
	assert.Equal(t, 0, reserved,
		"availableUUID не должна быть зарезервирована: транзакция откатилась")
}
