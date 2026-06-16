package tests

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_retry/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

type testUfo struct {
	id      int64
	title   string
	content string
}

// failingUFOServer сбоит первые failCount вызовов CreateUfo
type failingUFOServer struct {
	ufov1.UnimplementedUFOServiceServer
	mu          sync.RWMutex
	sightings   map[int64]*testUfo
	nextID      int64
	createCalls atomic.Int64
	failCount   int64
}

func newFailingUFOServer(failCount int64) *failingUFOServer {
	return &failingUFOServer{
		sightings: make(map[int64]*testUfo),
		nextID:    1,
		failCount: failCount,
	}
}

func (s *failingUFOServer) CreateUfo(_ context.Context, req *ufov1.CreateUfoRequest) (*ufov1.CreateUfoResponse, error) {
	callNum := s.createCalls.Add(1)

	if callNum <= s.failCount {
		return nil, status.Error(codes.Unavailable, "сервис временно недоступен")
	}

	if req.GetTitle() == "" {
		return nil, status.Error(codes.InvalidArgument, "заголовок обязателен")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	s.sightings[id] = &testUfo{id: id, title: req.GetTitle(), content: req.GetContent()}

	return &ufov1.CreateUfoResponse{Id: id}, nil
}

func (s *failingUFOServer) GetUfo(_ context.Context, req *ufov1.GetUfoRequest) (*ufov1.GetUfoResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, exists := s.sightings[req.GetId()]
	if !exists {
		return nil, status.Error(codes.NotFound, "наблюдение не найдено")
	}

	return &ufov1.GetUfoResponse{Id: n.id, Title: n.title, Content: n.content}, nil
}

func setupTest(t *testing.T, failCount int64, retryOpts ...grpcretry.CallOption) ufov1.UFOServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(grpcServer, newFailingUFOServer(failCount))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("ошибка: %v", err))
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpcretry.UnaryClientInterceptor(retryOpts...)),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close()
		grpcServer.GracefulStop()
	})

	return ufov1.NewUFOServiceClient(conn)
}

func TestRetry_SuccessAfterTemporaryFailure(t *testing.T) {
	// Сервер сбоит первые 2 вызова, 3-й проходит
	client := setupTest(
		t, 2,
		grpcretry.WithMax(3),
		grpcretry.WithBackoff(grpcretry.BackoffLinear(10*time.Millisecond)),
		grpcretry.WithCodes(codes.Unavailable),
	)

	resp, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "retry наблюдение",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.GetId(), int64(0))
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	// Сервер сбоит все вызовы — retry исчерпает попытки
	client := setupTest(
		t, 10,
		grpcretry.WithMax(3),
		grpcretry.WithBackoff(grpcretry.BackoffLinear(10*time.Millisecond)),
		grpcretry.WithCodes(codes.Unavailable),
	)

	_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "always fails",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestRetry_NoRetryForBusinessError(t *testing.T) {
	// Сервер не сбоит (failCount=0), но InvalidArgument — бизнес-ошибка, retry не нужен
	client := setupTest(
		t, 0,
		grpcretry.WithMax(3),
		grpcretry.WithBackoff(grpcretry.BackoffLinear(10*time.Millisecond)),
		grpcretry.WithCodes(codes.Unavailable),
	)

	_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "", // Пустой title -> InvalidArgument
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestRetry_ContextCancellation(t *testing.T) {
	// Сервер всегда сбоит, но контекст отменится раньше
	client := setupTest(
		t, 100,
		grpcretry.WithMax(5),
		grpcretry.WithBackoff(grpcretry.BackoffLinear(500*time.Millisecond)),
		grpcretry.WithCodes(codes.Unavailable),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := client.CreateUfo(ctx, &ufov1.CreateUfoRequest{
		Title: "cancelled",
	})
	require.Error(t, err)
	assert.True(
		t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
			status.Code(err) == codes.DeadlineExceeded,
		"ожидалась ошибка контекста, получено: %v", err,
	)
}

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	// Сервер не сбоит — retry не нужен, запрос проходит с первого раза
	client := setupTest(
		t, 0,
		grpcretry.WithMax(3),
		grpcretry.WithBackoff(grpcretry.BackoffLinear(10*time.Millisecond)),
		grpcretry.WithCodes(codes.Unavailable),
	)

	resp, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "сразу успех",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.GetId(), int64(0))
}

func TestRetry_PerCallOverride(t *testing.T) {
	// Глобально retry выключен (WithMax(1) = без повторов)
	// Но per-call override увеличивает до 3 попыток
	client := setupTest(
		t, 2,
		grpcretry.WithMax(1),
		grpcretry.WithBackoff(grpcretry.BackoffLinear(10*time.Millisecond)),
		grpcretry.WithCodes(codes.Unavailable),
	)

	// Без override — провал (только 1 попытка, а сервер сбоит 2 раза)
	_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "fail without override",
	})
	require.Error(t, err)

	// С per-call override на 5 попыток — успех
	// failCount=2, но createCalls уже = 1 от предыдущего вызова, поэтому нужно ещё 1 сбой + успех
	resp, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "success with override",
	}, grpcretry.WithMax(5))
	require.NoError(t, err)
	assert.Greater(t, resp.GetId(), int64(0))
}
