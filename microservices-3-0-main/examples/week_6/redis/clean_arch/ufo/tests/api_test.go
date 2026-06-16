package tests

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/api/ufo/v1"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/ufo"
	ufoCacheRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/repository/ufo_cache"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/service/ufo"
)

const bufSize = 1024 * 1024

var (
	lis       *bufconn.Listener
	ufoClient ufov1.UFOServiceClient
)

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func assertNotFoundError(t *testing.T, err error) {
	t.Helper()

	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code(), "неверный gRPC код: %s", st.Message())
}

func randomLocation() string {
	return fmt.Sprintf("Location %d", rand.IntN(1000000))
}

func randomDescription() string {
	return fmt.Sprintf("Description %d", rand.IntN(1000000))
}

func createTestSighting(t *testing.T) string {
	t.Helper()

	resp, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt:  timestamppb.Now(),
		Location:    randomLocation(),
		Description: randomDescription(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())

	return resp.GetUuid()
}

// TestMain запускает in-memory gRPC сервер через bufconn
// Использует testcontainers для PostgreSQL и Redis
func TestMain(m *testing.M) {
	_ = godotenv.Load("../../ufo.env")

	ctx := context.Background()

	// Поднимаем PostgreSQL через testcontainers
	pgContainer, err := tcpostgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		tcpostgres.WithDatabase("ufo"),
		tcpostgres.WithUsername("ufo_admin"),
		tcpostgres.WithPassword("ufo_secret"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось поднять PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	pgDSN, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить DSN для PostgreSQL", "error", err)
		os.Exit(1)
	}

	// Поднимаем Redis через testcontainers
	redisContainer, err := tcredis.Run(ctx, os.Getenv("REDIS_IMAGE"))
	if err != nil {
		slog.Error("не удалось поднять Redis контейнер", "error", err)
		os.Exit(1)
	}

	redisAddr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		slog.Error("не удалось получить адрес Redis", "error", err)
		os.Exit(1)
	}

	// Миграции через goose
	db, err := sql.Open("pgx", pgDSN)
	if err != nil {
		slog.Error("не удалось открыть sql.DB для миграций", "error", err)
		os.Exit(1)
	}

	if err = goose.Up(db, "../migrations"); err != nil {
		slog.Error("не удалось выполнить миграции", "error", err)
		os.Exit(1)
	}
	db.Close()

	// Подключаемся к PostgreSQL через pgxpool
	pool, err := pgxpool.New(ctx, pgDSN)
	if err != nil {
		slog.Error("не удалось создать пул подключений к PostgreSQL", "error", err)
		os.Exit(1)
	}

	// Создаём клиент Redis через go-redis
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	if err = rdb.Ping(ctx).Err(); err != nil {
		slog.Error("не удалось подключиться к Redis", "error", err)
		os.Exit(1)
	}

	_ = slog.Default() // используется для совместимости

	// DI инлайн
	repo := ufoRepository.NewRepository(pool)
	cacheRepo := ufoCacheRepository.NewRepository(rdb)
	svc := ufoService.New(repo, cacheRepo, 5*time.Minute)
	apiHandler := ufov1API.New(svc)

	// Запускаем gRPC сервер через bufconn
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(s, apiHandler)

	go func() {
		if err := s.Serve(lis); err != nil {
			panic("gRPC сервер завершился с ошибкой: " + err.Error())
		}
	}()

	// Создаём gRPC клиент через bufconn
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать gRPC клиент", "error", err)
		os.Exit(1)
	}
	ufoClient = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	s.Stop()
	conn.Close()
	rdb.Close()
	pool.Close()

	if termErr := pgContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить PostgreSQL контейнер", "error", termErr)
	}

	if termErr := redisContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить Redis контейнер", "error", termErr)
	}

	os.Exit(code)
}

// ============ Create Tests ============.

func TestCreate_Success(t *testing.T) {
	resp, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt:      timestamppb.Now(),
		Location:        randomLocation(),
		Description:     randomDescription(),
		Color:           wrapperspb.String("Green"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(120),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid())

	_, err = uuid.Parse(resp.GetUuid())
	require.NoError(t, err, "UUID должен быть валидным")

	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)
	assert.Equal(t, resp.GetUuid(), getResp.GetSighting().GetUuid())
	assert.NotNil(t, getResp.GetSighting().GetCreatedAt())
	assert.Nil(t, getResp.GetSighting().GetDeletedAt())
}

func TestCreate_MinimalFields(t *testing.T) {
	resp, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt:  timestamppb.Now(),
		Location:    randomLocation(),
		Description: randomDescription(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid())

	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)
	assert.Nil(t, getResp.GetSighting().GetColor())
	assert.Nil(t, getResp.GetSighting().GetSound())
	assert.Nil(t, getResp.GetSighting().GetDurationSeconds())
}

func TestCreate_UniqueUUIDs(t *testing.T) {
	resp1, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt: timestamppb.Now(), Location: randomLocation(), Description: randomDescription(),
	})
	require.NoError(t, err)

	resp2, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt: timestamppb.Now(), Location: randomLocation(), Description: randomDescription(),
	})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetUuid(), resp2.GetUuid())
}

// ============ Get Tests ============.

func TestGet_Success(t *testing.T) {
	createdUUID := createTestSighting(t)

	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, createdUUID, getResp.GetSighting().GetUuid())
}

func TestGet_NotFound(t *testing.T) {
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid.NewString()})
	assertNotFoundError(t, err)
}

func TestGet_EmptyUUID(t *testing.T) {
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: ""})
	assertNotFoundError(t, err)
}

// ============ Update Tests ============.

func TestUpdate_SingleField(t *testing.T) {
	createdUUID := createTestSighting(t)

	newLoc := randomLocation()
	_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String(newLoc),
	})
	require.NoError(t, err)

	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, newLoc, getResp.GetSighting().GetLocation())
}

func TestUpdate_MultipleFields(t *testing.T) {
	createdUUID := createTestSighting(t)

	newLoc := randomLocation()
	newDesc := randomDescription()
	_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid:        createdUUID,
		Location:    wrapperspb.String(newLoc),
		Description: wrapperspb.String(newDesc),
		Color:       wrapperspb.String("Red"),
	})
	require.NoError(t, err)

	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, newLoc, getResp.GetSighting().GetLocation())
	assert.Equal(t, newDesc, getResp.GetSighting().GetDescription())
	assert.Equal(t, "Red", getResp.GetSighting().GetColor().GetValue())
}

func TestUpdate_NotFound(t *testing.T) {
	_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid:     uuid.NewString(),
		Location: wrapperspb.String("X"),
	})
	assertNotFoundError(t, err)
}

func TestUpdate_NilUpdateInfo(t *testing.T) {
	createdUUID := createTestSighting(t)

	// Пустой UpdateRequest (без полей обновления) — не ошибка, просто ничего не меняет
	_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: createdUUID,
	})
	require.NoError(t, err)
}

func TestUpdate_UpdatesTimestamp(t *testing.T) {
	createdUUID := createTestSighting(t)

	getResp1, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	createdAt := getResp1.GetSighting().GetCreatedAt()

	_, err = ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String(randomLocation()),
	})
	require.NoError(t, err)

	getResp2, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	sighting := getResp2.GetSighting()
	assert.NotNil(t, sighting.GetUpdatedAt())
	assert.NotEqual(t, sighting.GetCreatedAt().AsTime(), sighting.GetUpdatedAt().AsTime(), "updated_at должен отличаться от created_at")
	assert.Equal(t, createdAt.AsTime().Unix(), sighting.GetCreatedAt().AsTime().Unix())
}

// ============ Delete Tests ============.

func TestDelete_Success(t *testing.T) {
	createdUUID := createTestSighting(t)

	_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// После soft-delete запись не доступна через Get (фильтр deleted_at IS NULL)
	_, err = ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	assertNotFoundError(t, err)
}

func TestDelete_NotFound(t *testing.T) {
	_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: uuid.NewString()})
	assertNotFoundError(t, err)
}

func TestDelete_AlreadyDeleted(t *testing.T) {
	createdUUID := createTestSighting(t)

	_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Повторное удаление должно вернуть NotFound (RowsAffected == 0)
	_, err = ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	assertNotFoundError(t, err)
}

// ============ Cache Tests ============.

func TestGet_CacheConsistency(t *testing.T) {
	createdUUID := createTestSighting(t)

	// Первый Get — из БД
	getResp1, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Второй Get — должен вернуть те же данные (из кеша)
	getResp2, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	assert.Equal(t, getResp1.GetSighting().GetUuid(), getResp2.GetSighting().GetUuid())
	assert.Equal(t, getResp1.GetSighting().GetLocation(), getResp2.GetSighting().GetLocation())
	assert.Equal(t, getResp1.GetSighting().GetDescription(), getResp2.GetSighting().GetDescription())
}

func TestUpdate_CacheInvalidation(t *testing.T) {
	createdUUID := createTestSighting(t)

	// Прогреваем кеш
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Обновляем
	newLoc := randomLocation()
	_, err = ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String(newLoc),
	})
	require.NoError(t, err)

	// Получаем — должны увидеть обновлённые данные
	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, newLoc, getResp.GetSighting().GetLocation())
}

func TestDelete_CacheInvalidation(t *testing.T) {
	createdUUID := createTestSighting(t)

	// Прогреваем кеш
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Удаляем
	_, err = ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// После удаления запись не доступна (кеш инвалидирован, из БД не вернётся)
	_, err = ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	assertNotFoundError(t, err)
}

// ============ Full Lifecycle Test ============.

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	createResp, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.Now(),
		Location:    randomLocation(),
		Description: randomDescription(),
	})
	require.NoError(t, err)
	id := createResp.GetUuid()

	getResp1, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	newLoc := randomLocation()
	_, err = ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     id,
		Location: wrapperspb.String(newLoc),
	})
	require.NoError(t, err)

	getResp2, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.Equal(t, newLoc, getResp2.GetSighting().GetLocation())

	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: id})
	require.NoError(t, err)

	// После удаления запись не доступна
	_, err = ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	assertNotFoundError(t, err)
}
