package testutil

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	invApp "github.com/student/inventory/pkg/app"
	"github.com/student/order/pkg/app"
	payApp "github.com/student/payment/pkg/app"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/student/shared/pkg/proto/payment/v1"
)

const bufSize = 1024 * 1024

// Env — изолированное тестовое окружение: свои БД, свои сервисы.
// Каждый параллельный тест получает свой Env и не пересекается с другими.
type Env struct {
	HTTPClient *http.Client
	BaseURL    string

	InventoryClient inventoryv1.InventoryServiceClient
	PaymentClient   paymentv1.PaymentServiceClient

	// Пулы прямого доступа к БД для проверок состояния и seed-данных.
	OrderPool     *pgxpool.Pool
	InventoryPool *pgxpool.Pool

	// Имена изолированных БД (полезно для отладки).
	OrderDBName     string
	InventoryDBName string
}

// NewEnv поднимает окружение для одного теста и регистрирует cleanup.
func NewEnv(t *testing.T) *Env {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	orderDB := createIsolatedDB(ctx, t, "order", "../../migrations/order")
	t.Cleanup(orderDB.cleanup)

	inventoryDB := createIsolatedDB(ctx, t, "inventory", "../../migrations/inventory")
	t.Cleanup(inventoryDB.cleanup)

	orderPool, err := pgxpool.New(ctx, orderDB.DSN)
	if err != nil {
		t.Fatalf("orderPool: %v", err)
	}
	t.Cleanup(orderPool.Close)

	inventoryPool, err := pgxpool.New(ctx, inventoryDB.DSN)
	if err != nil {
		t.Fatalf("inventoryPool: %v", err)
	}
	t.Cleanup(inventoryPool.Close)

	txManager, err := manager.New(trmpgx.NewDefaultFactory(orderPool))
	if err != nil {
		t.Fatalf("txManager: %v", err)
	}

	// Inventory gRPC через bufconn.
	invLis := bufconn.Listen(bufSize)
	invServer := grpc.NewServer(invApp.Interceptors()...)
	invApp.RegisterServices(invServer, inventoryPool)
	go func() { _ = invServer.Serve(invLis) }()
	t.Cleanup(invServer.Stop)

	invConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return invLis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("invConn: %v", err)
	}
	t.Cleanup(func() { _ = invConn.Close() })
	invClient := inventoryv1.NewInventoryServiceClient(invConn)

	// Payment gRPC через bufconn.
	payLis := bufconn.Listen(bufSize)
	payServer := grpc.NewServer(payApp.Interceptors()...)
	payApp.RegisterServices(payServer)
	go func() { _ = payServer.Serve(payLis) }()
	t.Cleanup(payServer.Stop)

	payConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return payLis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("payConn: %v", err)
	}
	t.Cleanup(func() { _ = payConn.Close() })
	payClient := paymentv1.NewPaymentServiceClient(payConn)

	// Order HTTP через httptest.
	orderHandler, err := app.NewHTTPHandler(orderPool, txManager, invClient, payClient)
	if err != nil {
		t.Fatalf("order handler: %v", err)
	}
	ts := httptest.NewServer(orderHandler)
	t.Cleanup(ts.Close)

	return &Env{
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		BaseURL:         ts.URL,
		InventoryClient: invClient,
		PaymentClient:   payClient,
		OrderPool:       orderPool,
		InventoryPool:   inventoryPool,
		OrderDBName:     orderDB.Name,
		InventoryDBName: inventoryDB.Name,
	}
}
