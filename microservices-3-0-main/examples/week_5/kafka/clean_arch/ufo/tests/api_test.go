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

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	wrappedKafkaProducer "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka/producer"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/shared/pkg/proto/ufo/v1"
	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/api/ufo/v1"
	ufoProducer "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/producer/ufo"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/service/ufo"
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

// assertNotFoundError проверяет, что ошибка содержит gRPC код NotFound
func assertNotFoundError(t *testing.T, err error) {
	t.Helper()

	assert.Error(t, err)

	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code(), "неверный gRPC код: %s", st.Message())
}

func randomLocation() string {
	return fmt.Sprintf("Локация %d", rand.IntN(1000000))
}

func randomDescription() string {
	return fmt.Sprintf("Описание %d", rand.IntN(1000000))
}

// createTestSighting создаёт наблюдение с минимальным набором полей и возвращает UUID
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

// createTestSightingFull создаёт наблюдение со всеми полями и возвращает UUID и исходный запрос
func createTestSightingFull(t *testing.T) (string, *ufov1.CreateRequest) {
	t.Helper()

	req := &ufov1.CreateRequest{
		ObservedAt:      timestamppb.Now(),
		Location:        randomLocation(),
		Description:     randomDescription(),
		Color:           wrapperspb.String("Зелёный"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(120),
	}

	resp, err := ufoClient.Create(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())

	return resp.GetUuid(), req
}

// TestMain поднимает PostgreSQL и Kafka через testcontainers, накатывает миграции
// и запускает in-memory gRPC сервер через bufconn
func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error

	// Читаем креды из ufo.env
	_ = godotenv.Load("../../ufo.env")

	// --- PostgreSQL через testcontainers ---
	pgContainer, err := postgres.Run(
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

	dbURI, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
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

	err = goose.Up(sqlDB, "../migrations")
	if err != nil {
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

	// --- Kafka через testcontainers ---
	kafkaContainer, err := kafka.Run(
		ctx, os.Getenv("KAFKA_IMAGE"),
		kafka.WithClusterID("test-cluster"),
	)
	if err != nil {
		slog.Error("не удалось запустить Kafka контейнер", "error", err)
		os.Exit(1)
	}

	kafkaBrokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		slog.Error("не удалось получить адрес Kafka брокера", "error", err)
		os.Exit(1)
	}

	kafkaTopic := "ufo.recorded"

	// Создаём Kafka SyncProducer
	syncProducer, err := sarama.NewSyncProducer(kafkaBrokers, nil)
	if err != nil {
		slog.Error("не удалось создать Kafka producer", "error", err)
		os.Exit(1)
	}

	// DI инлайн: repository → producer → service → api
	repo := ufoRepository.NewRepository(pool)
	wrappedProd := wrappedKafkaProducer.NewProducer(syncProducer, kafkaTopic)
	prodService := ufoProducer.NewService(wrappedProd)
	svc := ufoService.NewService(repo, prodService)
	api := ufov1API.NewAPI(svc)

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
	_ = syncProducer.Close()
	pool.Close()
	_ = pgContainer.Terminate(ctx)
	_ = kafkaContainer.Terminate(ctx)

	os.Exit(code)
}

// ==================== Тесты создания ====================.

func TestCreate(t *testing.T) {
	t.Run("все поля заполнены", func(t *testing.T) {
		req := &ufov1.CreateRequest{
			ObservedAt:      timestamppb.Now(),
			Location:        randomLocation(),
			Description:     randomDescription(),
			Color:           wrapperspb.String("Зелёный"),
			Sound:           wrapperspb.Bool(true),
			DurationSeconds: wrapperspb.Int32(120),
		}

		resp, err := ufoClient.Create(context.Background(), req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetUuid())

		_, parseErr := uuid.Parse(resp.GetUuid())
		require.NoError(t, parseErr, "UUID должен быть валидным")

		// Проверяем, что все поля корректно сохранены
		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: resp.GetUuid()})
		require.NoError(t, err)

		sighting := getResp.GetSighting()
		assert.Equal(t, resp.GetUuid(), sighting.GetUuid())
		assert.Equal(t, req.GetLocation(), sighting.GetLocation())
		assert.Equal(t, req.GetDescription(), sighting.GetDescription())
		assert.Equal(t, req.GetColor().GetValue(), sighting.GetColor().GetValue())
		assert.Equal(t, req.GetSound().GetValue(), sighting.GetSound().GetValue())
		assert.Equal(t, req.GetDurationSeconds().GetValue(), sighting.GetDurationSeconds().GetValue())
		assert.NotNil(t, sighting.GetCreatedAt())
		assert.Nil(t, sighting.GetUpdatedAt())
		assert.Nil(t, sighting.GetDeletedAt())
	})

	t.Run("минимальный набор полей", func(t *testing.T) {
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
	})

	t.Run("без observed_at", func(t *testing.T) {
		resp, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
			Location:    randomLocation(),
			Description: randomDescription(),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetUuid())

		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: resp.GetUuid()})
		require.NoError(t, err)
		assert.Nil(t, getResp.GetSighting().GetObservedAt())
	})

	t.Run("уникальность UUID", func(t *testing.T) {
		resp1, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
			ObservedAt: timestamppb.Now(), Location: randomLocation(), Description: randomDescription(),
		})
		require.NoError(t, err)

		resp2, err := ufoClient.Create(context.Background(), &ufov1.CreateRequest{
			ObservedAt: timestamppb.Now(), Location: randomLocation(), Description: randomDescription(),
		})
		require.NoError(t, err)

		assert.NotEqual(t, resp1.GetUuid(), resp2.GetUuid())
	})
}

// ==================== Тесты получения ====================.

func TestGet(t *testing.T) {
	t.Run("существующая запись", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		assert.Equal(t, createdUUID, getResp.GetSighting().GetUuid())
		assert.NotNil(t, getResp.GetSighting().GetCreatedAt())
	})

	t.Run("несуществующий UUID — NotFound", func(t *testing.T) {
		_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid.NewString()})
		assertNotFoundError(t, err)
	})

	t.Run("пустой UUID — NotFound", func(t *testing.T) {
		_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: ""})
		assertNotFoundError(t, err)
	})

	t.Run("удалённая запись всё ещё доступна", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
		require.NoError(t, err)

		// Get возвращает удалённую запись (soft-delete) — deleted_at установлен
		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		assert.Equal(t, createdUUID, getResp.GetSighting().GetUuid())
		assert.NotNil(t, getResp.GetSighting().GetDeletedAt())
	})
}

// ==================== Тесты обновления ====================.

func TestUpdate(t *testing.T) {
	t.Run("одно поле", func(t *testing.T) {
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
	})

	t.Run("несколько полей", func(t *testing.T) {
		createdUUID := createTestSighting(t)
		newLoc := randomLocation()
		newDesc := randomDescription()

		_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:        createdUUID,
			Location:    wrapperspb.String(newLoc),
			Description: wrapperspb.String(newDesc),
			Color:       wrapperspb.String("Красный"),
		})
		require.NoError(t, err)

		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		assert.Equal(t, newLoc, getResp.GetSighting().GetLocation())
		assert.Equal(t, newDesc, getResp.GetSighting().GetDescription())
		assert.Equal(t, "Красный", getResp.GetSighting().GetColor().GetValue())
	})

	t.Run("все optional поля", func(t *testing.T) {
		createdUUID := createTestSighting(t)
		newTime := timestamppb.New(time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))

		_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:            createdUUID,
			ObservedAt:      newTime,
			Location:        wrapperspb.String("Новая локация"),
			Description:     wrapperspb.String("Новое описание"),
			Color:           wrapperspb.String("Синий"),
			Sound:           wrapperspb.Bool(false),
			DurationSeconds: wrapperspb.Int32(999),
		})
		require.NoError(t, err)

		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)

		s := getResp.GetSighting()
		assert.Equal(t, newTime.AsTime().Unix(), s.GetObservedAt().AsTime().Unix())
		assert.Equal(t, "Новая локация", s.GetLocation())
		assert.Equal(t, "Новое описание", s.GetDescription())
		assert.Equal(t, "Синий", s.GetColor().GetValue())
		assert.Equal(t, false, s.GetSound().GetValue())
		assert.Equal(t, int32(999), s.GetDurationSeconds().GetValue())
	})

	t.Run("частичное обновление не затрагивает другие поля", func(t *testing.T) {
		createdUUID, originalReq := createTestSightingFull(t)

		// Обновляем только location
		newLoc := randomLocation()
		_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:     createdUUID,
			Location: wrapperspb.String(newLoc),
		})
		require.NoError(t, err)

		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)

		s := getResp.GetSighting()
		// Обновлённое поле изменилось
		assert.Equal(t, newLoc, s.GetLocation())
		// Остальные поля не изменились (COALESCE)
		assert.Equal(t, originalReq.GetDescription(), s.GetDescription())
		assert.Equal(t, originalReq.GetColor().GetValue(), s.GetColor().GetValue())
		assert.Equal(t, originalReq.GetSound().GetValue(), s.GetSound().GetValue())
		assert.Equal(t, originalReq.GetDurationSeconds().GetValue(), s.GetDurationSeconds().GetValue())
	})

	t.Run("updated_at устанавливается после обновления", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		getResp1, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		assert.Nil(t, getResp1.GetSighting().GetUpdatedAt(), "до обновления updated_at должен быть nil")

		createdAt := getResp1.GetSighting().GetCreatedAt()

		_, err = ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:     createdUUID,
			Location: wrapperspb.String(randomLocation()),
		})
		require.NoError(t, err)

		getResp2, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		sighting := getResp2.GetSighting()
		assert.NotNil(t, sighting.GetUpdatedAt(), "после обновления updated_at должен быть установлен")
		assert.NotEqual(t, sighting.GetCreatedAt().AsTime(), sighting.GetUpdatedAt().AsTime(), "updated_at должен отличаться от created_at")
		assert.Equal(t, createdAt.AsTime().Unix(), sighting.GetCreatedAt().AsTime().Unix(), "created_at не должен измениться")
	})

	t.Run("несуществующий UUID — NotFound", func(t *testing.T) {
		_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:     uuid.NewString(),
			Location: wrapperspb.String("X"),
		})
		assertNotFoundError(t, err)
	})

	t.Run("пустой UUID — NotFound", func(t *testing.T) {
		_, err := ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:     "",
			Location: wrapperspb.String("X"),
		})
		assertNotFoundError(t, err)
	})

	t.Run("удалённая запись — NotFound", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
		require.NoError(t, err)

		// UPDATE ... WHERE deleted_at IS NULL → 0 rows → NotFound
		_, err = ufoClient.Update(context.Background(), &ufov1.UpdateRequest{
			Uuid:     createdUUID,
			Location: wrapperspb.String("X"),
		})
		assertNotFoundError(t, err)
	})
}

// ==================== Тесты удаления ====================.

func TestDelete(t *testing.T) {
	t.Run("успешное удаление", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
		require.NoError(t, err)

		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		assert.NotNil(t, getResp.GetSighting().GetDeletedAt(), "после удаления deleted_at должен быть установлен")
	})

	t.Run("несуществующий UUID — NotFound", func(t *testing.T) {
		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: uuid.NewString()})
		assertNotFoundError(t, err)
	})

	t.Run("пустой UUID — NotFound", func(t *testing.T) {
		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: ""})
		assertNotFoundError(t, err)
	})

	t.Run("повторное удаление — NotFound", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
		require.NoError(t, err)

		// Повторный DELETE: WHERE deleted_at IS NULL → 0 rows → NotFound
		_, err = ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
		assertNotFoundError(t, err)
	})

	t.Run("Get после удаления возвращает запись", func(t *testing.T) {
		createdUUID := createTestSighting(t)

		_, err := ufoClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: createdUUID})
		require.NoError(t, err)

		// Soft-delete: запись остаётся в БД и доступна через Get
		getResp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: createdUUID})
		require.NoError(t, err)
		assert.Equal(t, createdUUID, getResp.GetSighting().GetUuid())
		assert.NotNil(t, getResp.GetSighting().GetDeletedAt())
	})
}

// ==================== Тест полного жизненного цикла ====================.

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Создание
	createResp, err := ufoClient.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:      timestamppb.Now(),
		Location:        randomLocation(),
		Description:     randomDescription(),
		Color:           wrapperspb.String("Белый"),
		Sound:           wrapperspb.Bool(false),
		DurationSeconds: wrapperspb.Int32(60),
	})
	require.NoError(t, err)

	id := createResp.GetUuid()

	// 2. Чтение — запись существует, не удалена, не обновлена
	getResp1, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())
	assert.Nil(t, getResp1.GetSighting().GetUpdatedAt())
	assert.Equal(t, "Белый", getResp1.GetSighting().GetColor().GetValue())

	// 3. Обновление
	newLoc := randomLocation()
	_, err = ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     id,
		Location: wrapperspb.String(newLoc),
		Color:    wrapperspb.String("Красный"),
	})
	require.NoError(t, err)

	// 4. Чтение после обновления — поля изменились, updated_at появился
	getResp2, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.Equal(t, newLoc, getResp2.GetSighting().GetLocation())
	assert.Equal(t, "Красный", getResp2.GetSighting().GetColor().GetValue())
	assert.NotNil(t, getResp2.GetSighting().GetUpdatedAt())
	assert.Nil(t, getResp2.GetSighting().GetDeletedAt())

	// 5. Удаление
	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: id})
	require.NoError(t, err)

	// 6. Чтение после удаления — deleted_at установлен
	getResp3, err := ufoClient.Get(ctx, &ufov1.GetRequest{Uuid: id})
	require.NoError(t, err)
	assert.NotNil(t, getResp3.GetSighting().GetDeletedAt())

	// 7. Обновление удалённой записи — NotFound
	_, err = ufoClient.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     id,
		Location: wrapperspb.String("X"),
	})
	assertNotFoundError(t, err)

	// 8. Повторное удаление — NotFound
	_, err = ufoClient.Delete(ctx, &ufov1.DeleteRequest{Uuid: id})
	assertNotFoundError(t, err)
}
