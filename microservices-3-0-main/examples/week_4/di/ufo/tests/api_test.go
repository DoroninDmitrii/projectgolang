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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/api/ufo/v1"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/service/ufo"
)

const bufSize = 1024 * 1024

var (
	// lis используется bufconn для in-memory gRPC соединений
	lis *bufconn.Listener

	// ufoClient gRPC клиент для тестов
	ufoClient ufov1.UFOServiceClient
)

// bufDialer возвращает in-memory соединение через bufconn
func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func assertGRPCStatus(t *testing.T, err error, expectedCode codes.Code) {
	t.Helper()
	if expectedCode == codes.OK {
		assert.NoError(t, err)
		return
	}
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, expectedCode, st.Code(), "неверный gRPC код: %s", st.Message())
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

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции
// и запускает in-memory gRPC сервер через bufconn
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Читаем креды из ufo.env
	_ = godotenv.Load("../../ufo.env")

	// Поднимаем PostgreSQL через testcontainers
	container, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase(os.Getenv("POSTGRES_DB")),
		postgres.WithUsername(os.Getenv("POSTGRES_USER")),
		postgres.WithPassword(os.Getenv("POSTGRES_PASSWORD")),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	dbURI, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// Накатываем миграции через goose
	sqlDB, err := sql.Open("pgx", dbURI)
	if err != nil {
		slog.Error("не удалось открыть sql.DB", "error", err)
		os.Exit(1)
	}

	_ = goose.SetDialect("postgres")
	if err = goose.Up(sqlDB, "../migrations"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}
	sqlDB.Close()

	// Подключаемся через pgxpool
	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// DI инлайн: repository → service → api
	repo := ufoRepository.New(pool)
	svc := ufoService.New(repo)
	api := ufov1API.New(svc)

	// Запускаем gRPC сервер через bufconn
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(s, api)

	go func() {
		if serveErr := s.Serve(lis); serveErr != nil {
			panic("gRPC сервер завершился с ошибкой: " + serveErr.Error())
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
	pool.Close()
	_ = container.Terminate(ctx)

	os.Exit(code)
}

// ============ Тесты создания ============.

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

func TestCreate_EmptyLocation(t *testing.T) {
	_, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{Location: ""})
	assertGRPCStatus(t, err, codes.InvalidArgument)
}

// ============ Тесты получения ============.

func TestGet_Success(t *testing.T) {
	createdUUID := createTestSighting(t)
	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, createdUUID, getResp.GetSighting().GetUuid())
}

func TestGet_NotFound(t *testing.T) {
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid.NewString()})
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGet_EmptyUUID(t *testing.T) {
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: ""})
	assertGRPCStatus(t, err, codes.NotFound)
}

// ============ Тесты обновления ============.

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
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestUpdate_NilUpdateInfo(t *testing.T) {
	createdUUID := createTestSighting(t)
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

// ============ Тесты удаления ============.

func TestDelete_Success(t *testing.T) {
	createdUUID := createTestSighting(t)
	_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotNil(t, getResp.GetSighting().GetDeletedAt())
}

func TestDelete_NotFound(t *testing.T) {
	_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: uuid.NewString()})
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestDelete_AlreadyDeleted(t *testing.T) {
	createdUUID := createTestSighting(t)

	_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Повторное удаление возвращает NotFound, т.к. WHERE deleted_at IS NULL не находит строку
	_, err = ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
	assertGRPCStatus(t, err, codes.NotFound)
}

// ============ Тест полного жизненного цикла ============.

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

	getResp3, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.NotNil(t, getResp3.GetSighting().GetDeletedAt())
}
