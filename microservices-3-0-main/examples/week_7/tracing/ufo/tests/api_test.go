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

	analysisv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/analysis/v1"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/api/ufo/v1"
	analysisClientPkg "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/client/grpc/analysis"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/service/ufo"
)

const bufSize = 1024 * 1024

var (
	analysisLis *bufconn.Listener
	ufoLis      *bufconn.Listener
	ufoClient   ufov1.UFOServiceClient
)

// fakeAnalysisServer - fake реализация AnalysisServiceServer для тестов
// Имитирует поведение реального Analysis сервиса: запрашивает наблюдение через UFO клиент,
// и если оно не найдено — пробрасывает ошибку
type fakeAnalysisServer struct {
	analysisv1.UnimplementedAnalysisServiceServer
	ufoClient ufov1.UFOServiceClient
}

func (f *fakeAnalysisServer) AnalyzeSighting(ctx context.Context, req *analysisv1.AnalyzeSightingRequest) (*analysisv1.AnalyzeSightingResponse, error) {
	_, err := f.ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: req.GetUuid()})
	if err != nil {
		return nil, err
	}

	return &analysisv1.AnalyzeSightingResponse{
		AnalysisResult:  "Фейковый результат анализа",
		Classification:  "Неизвестный летающий объект",
		ConfidenceScore: 0.85,
	}, nil
}

func analysisDialer(_ context.Context, _ string) (net.Conn, error) {
	return analysisLis.Dial()
}

func ufoDialer(_ context.Context, _ string) (net.Conn, error) {
	return ufoLis.Dial()
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

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции через goose
// и запускает UFO gRPC-сервис с fake Analysis сервером через bufconn
func TestMain(m *testing.M) {
	_ = godotenv.Load("../../ufo.env")

	ctx := context.Background()

	// 1. Поднимаем PostgreSQL через testcontainers
	pgContainer, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase("ufo"),
		postgres.WithUsername("ufo_admin"),
		postgres.WithPassword("ufo_secret"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	dbURI, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// 2. Накатываем миграции через goose
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

	// 3. Подключаемся через pgxpool
	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// 4. DI inline — UFO сервер (с pgxpool и analysis клиентом через bufconn)
	// Создаём listener'ы заранее — bufconn подключения ленивые, RPC вызовы
	// произойдут только когда оба сервера уже будут запущены
	ufoLis = bufconn.Listen(bufSize)
	analysisLis = bufconn.Listen(bufSize)

	analysisConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(analysisDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать Analysis клиент для UFO", "error", err)
		os.Exit(1)
	}

	repo := ufoRepository.NewRepository(pool)
	analysisClient := analysisClientPkg.New(analysisv1.NewAnalysisServiceClient(analysisConn))
	svc := ufoService.NewService(repo, analysisClient)
	api := ufov1API.NewAPI(svc)

	ufoSrv := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(ufoSrv, api)
	go ufoSrv.Serve(ufoLis) //nolint:errcheck // ошибка serve обрабатывается через graceful stop

	// 5. Fake Analysis сервер через bufconn (использует UFO клиент для проверки существования)
	ufoConnForAnalysis, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(ufoDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать UFO клиент для fake Analysis", "error", err)
		os.Exit(1)
	}

	analysisSrv := grpc.NewServer()
	analysisv1.RegisterAnalysisServiceServer(analysisSrv, &fakeAnalysisServer{
		ufoClient: ufov1.NewUFOServiceClient(ufoConnForAnalysis),
	})
	go analysisSrv.Serve(analysisLis) //nolint:errcheck // ошибка serve обрабатывается через graceful stop

	// 6. Создаём тестовый клиент
	testConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(ufoDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать тестовый gRPC клиент", "error", err)
		os.Exit(1)
	}
	ufoClient = ufov1.NewUFOServiceClient(testConn)

	code := m.Run()

	// Cleanup
	ufoSrv.Stop()
	analysisSrv.Stop()
	_ = testConn.Close()
	_ = ufoConnForAnalysis.Close()
	_ = analysisConn.Close()
	pool.Close()
	_ = pgContainer.Terminate(ctx)

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
	assertNotFoundError(t, err)
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
	assertNotFoundError(t, err)
}

// ============ Тесты анализа ============.

func TestAnalyzeSighting_Success(t *testing.T) {
	createdUUID := createTestSighting(t)

	resp, err := ufoClient.AnalyzeSighting(context.Background(), &ufov1.AnalyzeSightingRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetClassification())
	assert.Greater(t, resp.GetConfidenceScore(), float32(0))
}

func TestAnalyzeSighting_NotFound(t *testing.T) {
	_, err := ufoClient.AnalyzeSighting(context.Background(), &ufov1.AnalyzeSightingRequest{Uuid: uuid.NewString()})
	assertNotFoundError(t, err)
}

// ============ Тест полного жизненного цикла ============.

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	// Create
	createResp, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.Now(),
		Location:    randomLocation(),
		Description: randomDescription(),
	})
	require.NoError(t, err)
	id := createResp.GetUuid()

	// Get
	getResp1, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	// Update
	newLoc := randomLocation()
	_, err = ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     id,
		Location: wrapperspb.String(newLoc),
	})
	require.NoError(t, err)

	// Get — verify update
	getResp2, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.Equal(t, newLoc, getResp2.GetSighting().GetLocation())

	// Delete
	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: id})
	require.NoError(t, err)

	// Get — verify deleted
	getResp3, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.NotNil(t, getResp3.GetSighting().GetDeletedAt())
}
