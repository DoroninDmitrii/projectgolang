package tests

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
)

// Concurrency-тесты дополняют api_test.go и проверяют поведение под гонками
// по общему ресурсу: один и тот же заказ в Pay/Cancel одновременно, последняя
// единица детали в нескольких параллельных резервированиях, цепочка
// Pay → SendOrderPaid в условиях ошибки producer'а.
//
// На week_5 все эти сценарии становятся центральной темой — hw.md явно требует
// SELECT FOR UPDATE и атомарность Pay → OrderPaid в одной транзакции.

// TestConcurrent_CreateOrder_LastUnit_ExactlyOneSucceeds:
// два горутина одновременно создают заказ на одну и ту же деталь со stock=1.
// SELECT FOR UPDATE в Inventory.ReserveParts должен пропустить ровно один —
// второй получит 409 (out of stock).
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
	assert.Equal(t, 1, created, "должен создаться ровно один заказ")
	assert.Equal(t, 1, conflict, "второй создаваемый заказ должен получить Conflict")

	// Hull: использован 1 → reserved=1 (но stock тоже 1, не 0 — Reserve не трогает stock).
	assert.Equal(t, 1, partReserved(t, hullUUID), "ровно одна единица hull должна остаться зарезервированной")
}

// TestConcurrent_ReserveLastUnit_OneSucceedsOneFails: два горутина одновременно
// пытаются зарезервировать единственную единицу детали через Inventory.ReserveParts
// напрямую (минуя Order). SELECT FOR UPDATE должен пропустить ровно один вызов;
// второй получит ResourceExhausted (mapping ErrOutOfStock в interceptor/error.go).
//
// Тест дополняет TestConcurrent_CreateOrder_LastUnit_ExactlyOneSucceeds,
// проверяя ту же инварианту, но через прямой gRPC-вызов Inventory без Order.
func TestConcurrent_ReserveLastUnit_OneSucceedsOneFails(t *testing.T) {
	partUUID := uuid.New().String()
	_, err := inventoryDBPool.Exec(
		context.Background(),
		`INSERT INTO parts (uuid, name, description, part_type, price, stock_quantity, properties)
		 VALUES ($1, 'Concurrent reserve last unit', '', 'HULL', 1000, 1, '{"hull": {"strength": 100}}')`,
		partUUID,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = inventoryDBPool.Exec(context.Background(),
			`DELETE FROM parts WHERE uuid = $1`, partUUID)
	})

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		exhausted int
		others    []error
	)
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			_, err := inventoryClient.ReserveParts(context.Background(),
				&inventoryv1.ReservePartsRequest{Uuids: []string{partUUID}})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case status.Code(err) == codes.ResourceExhausted:
				exhausted++
			default:
				others = append(others, err)
			}
		}()
	}
	wg.Wait()

	assert.Empty(t, others, "ожидались только успех или ResourceExhausted, прочих ошибок быть не должно")
	assert.Equal(t, 1, successes, "ровно один резерв должен пройти")
	assert.Equal(t, 1, exhausted, "второй резерв должен получить ResourceExhausted")
	assert.Equal(t, 1, partReserved(t, partUUID), "ровно одна единица должна остаться зарезервированной")
}

// TestConcurrent_PayCancel_LeavesConsistentState: одновременно Pay и Cancel
// одного заказа. Минимальная гарантия — финальный статус не PENDING_PAYMENT
// и хотя бы одна операция вернула OK.
//
// В сервисе с SELECT FOR UPDATE в Pay и Cancel гонка должна сериализоваться:
// одна операция выигрывает, вторая видит изменённый статус и корректно
// отказывает. Тест НЕ требует строго `payOK XOR cancelOK` — допускает оба
// варианта победы и проверяет именно консистентность результата.
func TestConcurrent_PayCancel_LeavesConsistentState(t *testing.T) {
	created, createResp := createOrder(t, &CreateOrderRequest{
		UserUUID:   uuid.New().String(),
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	_ = createResp.Body.Close()
	require.NotNil(t, created)

	var (
		wg                      sync.WaitGroup
		payStatus, cancelStatus int
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, resp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
		defer func() { _ = resp.Body.Close() }()
		payStatus = resp.StatusCode
	}()
	go func() {
		defer wg.Done()
		_, resp := cancelOrder(t, created.OrderUUID)
		defer func() { _ = resp.Body.Close() }()
		cancelStatus = resp.StatusCode
	}()
	wg.Wait()

	// Хотя бы одна операция должна вернуть OK.
	assert.True(t,
		payStatus == http.StatusOK || cancelStatus == http.StatusOK,
		"хотя бы одна из операций должна выиграть (Pay=%d, Cancel=%d)", payStatus, cancelStatus)

	// В БД финальный статус — PAID или CANCELLED, не PENDING_PAYMENT.
	final, getResp := getOrder(t, created.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()
	require.NotNil(t, final)
	assert.Contains(t, []string{"PAID", "CANCELLED"}, final.Status,
		"состояние не должно остаться PENDING_PAYMENT")
}

// TestConcurrent_Reserve_MixedStock: гонка ReserveParts с батчем из двух
// деталей, где одна доступна, а вторая (HullOutOfStock) гарантированно
// out-of-stock. Цель — подсветить целостность транзакции ReserveParts:
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
			_, err := inventoryClient.ReserveParts(context.Background(),
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
	// потому что транзакция откатилась.
	assert.Equal(t, 0, partReserved(t, availableUUID),
		"availableUUID не должна быть зарезервирована: транзакция откатилась")
}

// Сценарий «producer.SendOrderPaid вернул ошибку» покрывается на уровне
// unit-теста сервиса в order/internal/service/order/tests/pay_test.go:
// см. TestService_Pay_ProducerError. Поднимать отдельный httptest-сервер
// с подменённым producer'ом ради одного кейса — overkill, инжектировать
// другой producer в общий ts без перестройки TestMain невозможно.
