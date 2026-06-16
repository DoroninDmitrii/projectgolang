package tests

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/student/order/tests/testutil"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
)

// Concurrency-тест проверяет атомарность батча ReserveParts при mixed-stock.
// Каждый тест получает собственное окружение через testutil.NewEnv, поэтому
// конкуренция возникает только между горутинами внутри теста, не между тестами.
//
// Сценарии «ровно один победит» (последняя единица детали в двух одновременных
// резервированиях / создании заказов, гонка Pay/Cancel одного заказа) здесь
// не проверяются: текущий код делает read-modify-write без SELECT FOR UPDATE,
// поэтому такая инвариантa не является контрактом week_4. Эта тема (SELECT
// FOR UPDATE, optimistic CAS) появляется в week_5+, и соответствующие тесты
// живут там.

// TestConcurrent_Reserve_MixedStock: гонка ReserveParts с батчем из двух
// деталей, где одна доступна, а вторая (HullOutOfStock) гарантированно
// out-of-stock (stock=0 в seed). Цель — показать целостность транзакции
// ReserveParts: операция атомарна, и при провале хотя бы одной детали
// никакие резервы не сохраняются.
func TestConcurrent_Reserve_MixedStock(t *testing.T) {
	t.Parallel()
	env := testutil.NewEnv(t)

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
			_, err := env.InventoryClient.ReserveParts(context.Background(),
				&inventoryv1.ReservePartsRequest{
					Uuids: []string{testutil.HullAluminumUUID, testutil.HullOutOfStockUUID},
				})
			require.Error(t, err)
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

	// Главное: ни одна деталь из батча не должна быть зарезервирована,
	// потому что транзакция откатилась целиком.
	env.AssertPartReserved(t, testutil.HullAluminumUUID, 0)
	env.AssertPartReserved(t, testutil.HullOutOfStockUUID, 0)
}
