package tests

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_circuit_breaker/internal/api"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_circuit_breaker/internal/circuitbreaker"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_circuit_breaker/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

func setupTest(t *testing.T, cb *gobreaker.CircuitBreaker[any]) (ufov1.UFOServiceClient, ufov1.UFOServiceClient) {
	t.Helper()

	srv := api.NewAPI()
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(grpcServer, srv)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("ошибка: %v", err))
		}
	}()

	// Клиент с circuit breaker для тестирования
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(circuitbreaker.NewClientInterceptor(cb)),
	)
	require.NoError(t, err)

	// Прямой клиент без circuit breaker для управления режимом сбоев
	directConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close()
		directConn.Close()
		grpcServer.GracefulStop()
	})

	return ufov1.NewUFOServiceClient(conn), ufov1.NewUFOServiceClient(directConn)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	client, _ := setupTest(t, gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name: "test-closed",
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	}))

	// В закрытом состоянии запросы проходят
	resp, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "Тестовое наблюдение",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.GetId(), int64(0))
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	client, direct := setupTest(t, gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name: "test-open",
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	}))

	// Включаем режим сбоев
	_, err := direct.SetFailMode(context.Background(), &ufov1.SetFailModeRequest{Enabled: true})
	require.NoError(t, err)

	// Вызываем 3 раза — порог для открытия
	for range 3 {
		_, err = client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "test"})
		require.Error(t, err)
	}

	// Следующий вызов должен быть мгновенно отклонён circuit breaker
	_, err = client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "test"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code(), "circuit breaker должен вернуть Unavailable")
}

func TestCircuitBreaker_BusinessErrorsIgnored(t *testing.T) {
	client, _ := setupTest(t, gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name: "test-business",
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	}))

	// Бизнес-ошибки (InvalidArgument) не должны открывать circuit breaker
	for range 5 {
		_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: ""})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	}

	// Circuit breaker всё ещё закрыт — нормальный запрос проходит
	resp, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "работает"})
	require.NoError(t, err)
	assert.Greater(t, resp.GetId(), int64(0))
}

func TestCircuitBreaker_Recovery(t *testing.T) {
	client, direct := setupTest(t, gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        "test-recovery",
		MaxRequests: 1,
		Timeout:     1 * time.Second, // Быстрый timeout для тестов
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	}))

	// Включаем сбои и открываем circuit breaker
	_, err := direct.SetFailMode(context.Background(), &ufov1.SetFailModeRequest{Enabled: true})
	require.NoError(t, err)

	for range 2 {
		_, _ = client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "fail"})
	}

	// Проверяем, что circuit breaker открыт
	_, err = client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "blocked"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())

	// Выключаем сбои и ждём timeout для перехода в HalfOpen
	_, err = direct.SetFailMode(context.Background(), &ufov1.SetFailModeRequest{Enabled: false})
	require.NoError(t, err)
	time.Sleep(1500 * time.Millisecond) //nolint:forbidigo // тестовое ожидание таймаута circuit breaker

	// Запрос должен пройти (HalfOpen -> Closed)
	resp, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "восстановление"})
	require.NoError(t, err)
	assert.Greater(t, resp.GetId(), int64(0))
}
