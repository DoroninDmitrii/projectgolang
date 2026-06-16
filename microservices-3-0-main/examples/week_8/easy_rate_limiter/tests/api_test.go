package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/internal/ratelimit"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

type testUFOServer struct {
	ufov1.UnimplementedUFOServiceServer
	mu        sync.RWMutex
	sightings map[int64]*testUfo
	nextID    int64
}

type testUfo struct {
	id      int64
	title   string
	content string
}

func newTestUFOServer() *testUFOServer {
	return &testUFOServer{
		sightings: make(map[int64]*testUfo),
		nextID:    1,
	}
}

func (s *testUFOServer) CreateUfo(_ context.Context, req *ufov1.CreateUfoRequest) (*ufov1.CreateUfoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	s.sightings[id] = &testUfo{id: id, title: req.GetTitle(), content: req.GetContent()}

	return &ufov1.CreateUfoResponse{Id: id}, nil
}

func (s *testUFOServer) GetUfo(_ context.Context, req *ufov1.GetUfoRequest) (*ufov1.GetUfoResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, exists := s.sightings[req.GetId()]
	if !exists {
		return nil, status.Error(codes.NotFound, "наблюдение не найдено")
	}

	return &ufov1.GetUfoResponse{Id: n.id, Title: n.title, Content: n.content}, nil
}

// Используем маленький burst для тестирования rate limiter
const (
	testRPS   = 1.0
	testBurst = 3
)

var grpcClient ufov1.UFOServiceClient

func TestMain(m *testing.M) {
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(ratelimit.GRPCInterceptor(rate.NewLimiter(rate.Limit(testRPS), testBurst))),
	)
	ufov1.RegisterUFOServiceServer(grpcServer, newTestUFOServer())

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("ошибка gRPC сервера: %v", err))
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(fmt.Sprintf("ошибка подключения: %v", err))
	}

	grpcClient = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	conn.Close()
	grpcServer.GracefulStop()

	os.Exit(code)
}

func TestGRPC_RateLimit_BurstAllowed(t *testing.T) {
	// Первые burst запросов должны пройти
	for i := range testBurst {
		_, err := grpcClient.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
			Title: "наблюдение",
		})
		assert.NoError(t, err, "запрос %d из burst должен пройти", i+1)
	}
}

func TestGRPC_RateLimit_ExceedBurst(t *testing.T) {
	// Создаём новый сервер с отдельным limiter для чистого теста
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(ratelimit.GRPCInterceptor(rate.NewLimiter(0.001, 2))), // Очень низкий RPS
	)
	ufov1.RegisterUFOServiceServer(grpcServer, newTestUFOServer())

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("ошибка: %v", err))
		}
	}()
	defer grpcServer.GracefulStop()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	cl := ufov1.NewUFOServiceClient(conn)

	// Исчерпываем burst
	for range 2 {
		_, err = cl.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "test"})
		require.NoError(t, err)
	}

	// Следующий запрос должен быть отклонён
	_, err = cl.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "too many"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestHTTP_RateLimit_BurstAllowed(t *testing.T) {
	r := chi.NewRouter()
	r.Use(ratelimit.HTTPMiddleware(rate.NewLimiter(0.001, 3))) // Очень низкий RPS, burst=3.
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck // test
	})

	// Первые burst запросов должны пройти
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "запрос %d должен пройти", i+1)
	}

	// Следующий запрос — 429.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestHTTP_RateLimit_ExceedReturns429(t *testing.T) {
	r := chi.NewRouter()
	r.Use(ratelimit.HTTPMiddleware(rate.NewLimiter(0.001, 1))) // burst=1.
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Первый запрос — OK
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Второй запрос — 429.
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}
