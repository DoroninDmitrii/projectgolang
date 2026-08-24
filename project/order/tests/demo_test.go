//go:build demo

// Package tests, демо-часть: тесты для кейсов из MUTEX.md и CONTEXT.md.
//
// Файл собирается только с тегом demo, чтобы не замедлять обычный прогон:
//
//	cd project/order
//	go test -tags demo ./tests/ -race -run TestDemoParallelCreate -count=1 -v
//
// Сами по себе, на исправном коде, эти тесты проходят. Смысл появляется, когда
// ты вносишь правку из документа в handler.go и запускаешь их снова — тогда
// видно разницу «до» и «после». Какой тест к какому кейсу:
//
//	TestDemoParallelCreate  — MUTEX.md, кейсы 1, 2, 3 (гонка на map)
//	TestDemoPayThenGet      — MUTEX.md, кейс 5 (заказ из map — это копия)
//	TestDemoDoublePay       — MUTEX.md, кейс 7 (check-then-write)
//	TestDemoClientGoesAway  — CONTEXT.md, кейс 4 (цепочка отмены)
//
// Стенд (Inventory, Payment, Order) поднимает TestMain из api_test.go — ни Docker,
// ни запущенные сервисы не нужны.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestDemoParallelCreate шлёт 30 параллельных POST /api/v1/orders.
//
// MUTEX.md, кейсы 1-3. Закомментируй Lock/Unlock в handler.go:203-205
// (или замени Lock на RLock) и запусти с -race.
func TestDemoParallelCreate(t *testing.T) {
	const n = 30

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			createOrder(t, &CreateOrderRequest{
				HullUUID:   HullAluminumUUID,
				EngineUUID: EngineIonCUUID,
			})
		}()
	}
	wg.Wait()
}

// TestDemoPayThenGet создаёт заказ, оплачивает его и читает обратно.
//
// MUTEX.md, кейс 5. Удали строку h.store.orders[params.OrderUUID] = order
// в конце PayOrder (handler.go:263) и сравни вывод с текущим.
func TestDemoPayThenGet(t *testing.T) {
	created, resp := createOrder(t, &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	t.Logf("создан заказ: HTTP %d uuid=%s", resp.StatusCode, created.OrderUUID)

	paid, payResp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	t.Logf("оплата: HTTP %d transaction_uuid=%v", payResp.StatusCode, paid)

	dto, getResp := getOrder(t, created.OrderUUID)
	t.Logf("GET после оплаты: HTTP %d status=%q transaction_uuid=%v",
		getResp.StatusCode, dto.Status, dto.TransactionUUID)
}

// TestDemoDoublePay пытается оплатить один заказ двумя параллельными запросами.
//
// MUTEX.md, кейс 7. На текущем коде (единый Lock на весь PayOrder) второй запрос
// получает 409. Разбей замок на RLock...RUnlock + Lock — и оба получат 200.
func TestDemoDoublePay(t *testing.T) {
	created, _ := createOrder(t, &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, resp := payOrder(t, created.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
			t.Logf("оплата вернула HTTP %d", resp.StatusCode)
		}()
	}
	wg.Wait()
}

// TestDemoClientGoesAway обрывает запрос через 500 мс.
//
// CONTEXT.md, кейс 4. Без задержки в inventory/pkg/service/service.go тест ничего
// не покажет: запрос успевает отработать до того, как клиент уйдёт. Сначала добавь
// туда select с time.After(3s) из документа, потом сравни два прогона —
// с ctx и с context.Background() в вызове ListParts (handler.go:163).
func TestDemoClientGoesAway(t *testing.T) {
	body, err := json.Marshal(&CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		orderBaseURL()+"/api/v1/orders", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := (&http.Client{}).Do(req)
	t.Logf("КЛИЕНТ ушёл через %v, err=%v", time.Since(start).Round(10*time.Millisecond), err)
	if err == nil {
		resp.Body.Close()
	}

	// даём серверу договорить в лог
	time.Sleep(4 * time.Second)
}
