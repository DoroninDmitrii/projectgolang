package tests

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/go-redis/redis_rate/v10"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/internal/ratelimit"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

var redisAddr string

type testUfo struct {
	id      int64
	title   string
	content string
}

type testUFOServer struct {
	ufov1.UnimplementedUFOServiceServer
	mu        sync.RWMutex
	sightings map[int64]*testUfo
	nextID    int64
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

func (s *testUFOServer) ListUfo(_ context.Context, _ *ufov1.ListUfoRequest) (*ufov1.ListUfoResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ufos := make([]*ufov1.ListUfoResponseUfo, 0, len(s.sightings))
	for _, n := range s.sightings {
		ufos = append(ufos, &ufov1.ListUfoResponseUfo{
			Id:      n.id,
			Title:   n.title,
			Content: n.content,
		})
	}

	return &ufov1.ListUfoResponse{Ufos: ufos}, nil
}

func TestMain(m *testing.M) {
	_ = godotenv.Load("../.env")

	ctx := context.Background()

	// Поднимаем Redis через testcontainers
	redisContainer, err := tcredis.Run(ctx, os.Getenv("REDIS_IMAGE"))
	if err != nil {
		slog.Error("не удалось запустить Redis контейнер", "error", err)
		os.Exit(1)
	}

	host, err := redisContainer.Host(ctx)
	if err != nil {
		slog.Error("не удалось получить хост Redis контейнера", "error", err)
		os.Exit(1)
	}

	port, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		slog.Error("не удалось получить порт Redis контейнера", "error", err)
		os.Exit(1)
	}

	redisAddr = fmt.Sprintf("%s:%s", host, port.Port())

	code := m.Run()

	_ = redisContainer.Terminate(ctx)
	os.Exit(code)
}

// setupServer создаёт gRPC-сервер с распределённым rate limiter и возвращает клиент
func setupServer(
	t *testing.T,
	defaultLimit redis_rate.Limit,
	methodLimits map[string]redis_rate.Limit,
) ufov1.UFOServiceClient {
	t.Helper()

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	t.Cleanup(func() { rdb.Close() })

	// Очищаем ключи rate limiter перед каждым тестом
	// redis_rate добавляет внутренний префикс "rate:" к ключу (info.FullMethod), итого "rate:/ufo.v1.UFOService/*".
	ctx := context.Background()
	keys, err := rdb.Keys(ctx, "rate:*").Result()
	require.NoError(t, err)
	if len(keys) > 0 {
		require.NoError(t, rdb.Del(ctx, keys...).Err())
	}

	limiter := redis_rate.NewLimiter(rdb)

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(ratelimit.NewGRPCInterceptor(limiter, defaultLimit, methodLimits)),
	)
	ufov1.RegisterUFOServiceServer(grpcServer, newTestUFOServer())

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			panic(fmt.Sprintf("ошибка gRPC сервера: %v", serveErr))
		}
	}()
	t.Cleanup(func() { grpcServer.GracefulStop() })

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return ufov1.NewUFOServiceClient(conn)
}

func TestDistributedRateLimit_DefaultLimit(t *testing.T) {
	// Лимит: 2 запроса в секунду (burst=2 через PerSecond)
	client := setupServer(t, redis_rate.PerSecond(2), nil)

	// Первые 2 запроса должны пройти (burst)
	for i := range 2 {
		_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
			Title: "наблюдение",
		})
		assert.NoError(t, err, "запрос %d должен пройти", i+1)
	}

	// Третий запрос должен быть отклонён
	_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{
		Title: "лишний запрос",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestDistributedRateLimit_PerMethodLimits(t *testing.T) {
	// CreateUfo: 1 req/s, GetUfo: 10 req/s, ListUfo: default (2 req/s)
	methodLimits := map[string]redis_rate.Limit{
		"/ufo.v1.UFOService/CreateUfo": redis_rate.PerSecond(1),
		"/ufo.v1.UFOService/GetUfo":    redis_rate.PerSecond(10),
	}
	client := setupServer(t, redis_rate.PerSecond(2), methodLimits)

	// CreateUfo: первый запрос проходит, второй — нет
	_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "ok"})
	require.NoError(t, err)

	_, err = client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "rejected"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())

	// GetUfo: должен пройти свободно (лимит 10 req/s, мы делаем всего несколько)
	for i := range 5 {
		_, err = client.GetUfo(context.Background(), &ufov1.GetUfoRequest{Id: 999})
		// Ожидаем NotFound (наблюдение не существует), но НЕ ResourceExhausted
		if err != nil {
			st, ok = status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.NotFound, st.Code(), "GetUfo запрос %d не должен быть отклонён rate limiter", i+1)
		}
	}

	// ListUfo: использует defaultLimit (2 req/s). Первые 2 проходят, третий — нет
	for i := range 2 {
		_, err = client.ListUfo(context.Background(), &ufov1.ListUfoRequest{})
		assert.NoError(t, err, "ListUfo запрос %d должен пройти (default limit)", i+1)
	}

	_, err = client.ListUfo(context.Background(), &ufov1.ListUfoRequest{})
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code(), "ListUfo должен быть ограничен default limit")
}

func TestDistributedRateLimit_ResourceExhaustedMessage(t *testing.T) {
	client := setupServer(t, redis_rate.PerSecond(1), nil)

	// Исчерпываем лимит
	_, err := client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "first"})
	require.NoError(t, err)

	// Проверяем сообщение об ошибке
	_, err = client.CreateUfo(context.Background(), &ufov1.CreateUfoRequest{Title: "second"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Contains(t, st.Message(), "слишком много запросов")
}
