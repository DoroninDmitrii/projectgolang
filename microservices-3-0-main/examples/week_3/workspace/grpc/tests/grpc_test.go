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
	_ "github.com/jackc/pgx/v5/stdlib" // pgx как database/sql драйвер для goose
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

	ufoAPI "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/api/ufo/v1"
	ufoRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/repository/ufo"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

var (
	// lis используется bufconn для in-memory gRPC соединений
	lis *bufconn.Listener

	// ufoClient gRPC клиент для тестов UFO Service
	ufoClient ufov1.UFOServiceClient
)

// bufDialer возвращает in-memory соединение через bufconn
func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

// assertGRPCStatus проверяет, что ошибка является gRPC ошибкой с указанным кодом
func assertGRPCStatus(t *testing.T, err error, expectedCode codes.Code) {
	t.Helper()

	if expectedCode == codes.OK {
		assert.NoError(t, err, "ожидалось отсутствие ошибки")
		return
	}

	assert.Error(t, err, "ожидалась ошибка")

	st, ok := status.FromError(err)
	assert.True(t, ok, "ошибка должна быть gRPC статусом")
	assert.Equal(t, expectedCode, st.Code(), "неверный gRPC код: %s", st.Message())
}

// randomCreateRequest создаёт CreateRequest со случайными данными
func randomCreateRequest() *ufov1.CreateRequest {
	return &ufov1.CreateRequest{
		ObservedAt:  timestamppb.Now(),
		Location:    fmt.Sprintf("Test Location %d", rand.IntN(10000)),
		Description: fmt.Sprintf("Test Description %d", rand.IntN(10000)),
	}
}

// createTestSighting создаёт тестовую запись и возвращает её UUID
func createTestSighting(t *testing.T, location, description string) string {
	t.Helper()

	resp, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    location,
		Description: description,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())

	return resp.GetUuid()
}

// TestMain запускает тестовый PostgreSQL контейнер и gRPC сервер перед тестами
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Читаем креды из .env
	_ = godotenv.Load("../../grpc.env")

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

	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("не удалось установить диалект goose", "error", err)
		os.Exit(1)
	}

	if err = goose.Up(sqlDB, "../../migrations/grpc"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}

	sqlDB.Close()

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// DI инлайн: repository → api
	repo := ufoRepo.NewRepository(pool)
	api := ufoAPI.NewAPI(repo)

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

// Create Tests

func TestCreate_Success(t *testing.T) {
	ctx := context.Background()

	observedAt := time.Now().Truncate(time.Microsecond)

	resp, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:      timestamppb.New(observedAt),
		Location:        "Test Location",
		Description:     "Test Description",
		Color:           wrapperspb.String("Green"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(120),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid(), "UUID должен быть установлен")

	_, err = uuid.Parse(resp.GetUuid())
	require.NoError(t, err, "UUID должен быть валидным")

	// Проверяем, что все данные сохранились корректно
	getResp, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, resp.GetUuid(), sighting.GetUuid())
	assert.Equal(t, "Test Location", sighting.GetLocation())
	assert.Equal(t, "Test Description", sighting.GetDescription())
	assert.Equal(t, "Green", sighting.GetColor().GetValue())
	assert.True(t, sighting.GetSound().GetValue())
	assert.Equal(t, int32(120), sighting.GetDurationSeconds().GetValue())
	assert.NotNil(t, sighting.GetCreatedAt(), "created_at должен быть установлен")
	assert.Nil(t, sighting.GetDeletedAt(), "deleted_at должен быть nil для новой записи")
}

func TestCreate_MinimalFields(t *testing.T) {
	ctx := context.Background()

	resp, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    "Minimal Location",
		Description: "Minimal Description",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid())

	// Проверяем, что опциональные поля nil
	getResp, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	assert.Nil(t, getResp.GetSighting().GetColor())
	assert.Nil(t, getResp.GetSighting().GetSound())
	assert.Nil(t, getResp.GetSighting().GetDurationSeconds())
}

func TestCreate_UniqueUUIDs(t *testing.T) {
	ctx := context.Background()

	resp1, err := ufoClient.Create(ctx, randomCreateRequest())
	require.NoError(t, err)

	resp2, err := ufoClient.Create(ctx, randomCreateRequest())
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetUuid(), resp2.GetUuid(), "UUID должны быть уникальными")
}

func TestCreate_EmptyLocation(t *testing.T) {
	ctx := context.Background()

	_, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		Location: "",
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

// Get Tests

func TestGet_Success(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "Get Test Location", "Get Test Description")

	getResp, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, createdUUID, sighting.GetUuid())
	assert.Equal(t, "Get Test Location", sighting.GetLocation())
	assert.Equal(t, "Get Test Description", sighting.GetDescription())
	assert.NotNil(t, sighting.GetCreatedAt())
}

func TestGet_NotFound(t *testing.T) {
	ctx := context.Background()

	_, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: uuid.NewString()})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGet_EmptyUUID(t *testing.T) {
	ctx := context.Background()

	_, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: ""})

	assertGRPCStatus(t, err, codes.NotFound)
}

// Update Tests

func TestUpdate_SingleField(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "Original Location", "Original Description")

	_, err := ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("Updated Location"),
	})
	require.NoError(t, err)

	getResp, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	assert.Equal(t, "Updated Location", getResp.GetSighting().GetLocation())
	assert.Equal(t, "Original Description", getResp.GetSighting().GetDescription())
}

func TestUpdate_MultipleFields(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "Original Location", "Original Description")

	_, err := ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:        createdUUID,
		Location:    wrapperspb.String("Updated Location"),
		Description: wrapperspb.String("Updated Description"),
		Color:       wrapperspb.String("Blue"),
	})
	require.NoError(t, err)

	getResp, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "Updated Location", sighting.GetLocation())
	assert.Equal(t, "Updated Description", sighting.GetDescription())
	assert.Equal(t, "Blue", sighting.GetColor().GetValue())
}

func TestUpdate_NotFound(t *testing.T) {
	ctx := context.Background()

	_, err := ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     uuid.NewString(),
		Location: wrapperspb.String("New Location"),
	})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestUpdate_NilUpdateInfo(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "Location", "Description")

	_, err := ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid: createdUUID,
	})

	// Пустой update — no-op, не ошибка
	require.NoError(t, err)
}

func TestUpdate_UpdatesTimestamp(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "Location", "Description")

	// Получаем исходные данные
	getResp1, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	createdAt := getResp1.GetSighting().GetCreatedAt()

	// Обновляем
	_, err = ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("New Location"),
	})
	require.NoError(t, err)

	// Проверяем состояние после обновления
	getResp2, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp2.GetSighting()
	assert.NotNil(t, sighting.GetUpdatedAt(), "updated_at должен быть установлен после обновления")
	assert.Equal(t, "New Location", sighting.GetLocation())
	assert.Equal(t, createdAt.AsTime().Unix(), sighting.GetCreatedAt().AsTime().Unix(), "created_at не должен измениться")
}

// Delete Tests

func TestDelete_Success(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "To Delete Location", "To Delete Description")

	// Проверяем состояние до удаления
	getResp1, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt(), "deleted_at должен быть nil до удаления")

	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Проверяем состояние после удаления
	getResp2, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	assert.NotNil(t, getResp2.GetSighting().GetDeletedAt(), "deleted_at должен быть установлен")
}

func TestDelete_NotFound(t *testing.T) {
	ctx := context.Background()

	_, err := ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: uuid.NewString()})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestDelete_AlreadyDeleted(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, "Double Delete", "Double Delete")

	// Первое удаление
	_, err := ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Проверяем, что deleted_at установлен
	getResp, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotNil(t, getResp.GetSighting().GetDeletedAt())

	// Повторное удаление — запись уже помечена как удалённая (deleted_at IS NOT NULL),
	// поэтому WHERE ... AND deleted_at IS NULL не находит строку → NotFound
	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	assertGRPCStatus(t, err, codes.NotFound)
}

// Full Lifecycle Test

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Create
	createResp, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:      timestamppb.New(time.Now()),
		Location:        "Lifecycle Location",
		Description:     "Lifecycle Description",
		Color:           wrapperspb.String("blue"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(120),
	})
	require.NoError(t, err)
	createdUUID := createResp.GetUuid()
	assert.NotEmpty(t, createdUUID)

	// 2. Get после создания
	getResp1, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle Location", getResp1.GetSighting().GetLocation())
	assert.NotNil(t, getResp1.GetSighting().GetCreatedAt())
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	// 3. Update
	_, err = ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("Updated Lifecycle Location"),
	})
	require.NoError(t, err)

	// 4. Get после обновления
	getResp2, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, "Updated Lifecycle Location", getResp2.GetSighting().GetLocation())
	assert.NotNil(t, getResp2.GetSighting().GetUpdatedAt())

	// 5. Delete
	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// 6. Get после удаления
	getResp3, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotNil(t, getResp3.GetSighting().GetDeletedAt(), "запись должна быть помечена как удалённая")
}
