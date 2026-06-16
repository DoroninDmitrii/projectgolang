package tests

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ufov1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/api/ufo/v1"
	weatherStub "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/client/stub/weather"
	ufoRepository "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/repository/ufo"
	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/service/ufo"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

var (
	lis    *bufconn.Listener
	client ufov1.UFOServiceClient
)

// TestMain запускает in-memory gRPC сервер через bufconn перед тестами
// Использует DI-цепочку: repository -> service -> api
func TestMain(m *testing.M) {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()

	// DI: собираем сервис как в main.go
	repo := ufoRepository.NewRepository()
	weatherClient := weatherStub.New()
	service := ufoService.NewService(repo, weatherClient)
	api := ufov1API.NewAPI(service)

	ufov1.RegisterUFOServiceServer(s, api)

	go func() {
		if err := s.Serve(lis); err != nil {
			panic(err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать gRPC клиент", "error", err)
		os.Exit(1)
	}
	client = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	conn.Close()
	s.Stop()
	os.Exit(code)
}

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

// createTestSighting создаёт тестовую запись и возвращает её UUID
func createTestSighting(t *testing.T, client ufov1.UFOServiceClient, location, description string) string {
	t.Helper()

	resp, err := client.Create(context.Background(), &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    location,
		Description: description,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())

	return resp.GetUuid()
}

// ============ Create Tests ============.

func TestCreate_Success(t *testing.T) {
	ctx := context.Background()

	observedAt := time.Now().Truncate(time.Microsecond)

	resp, err := client.Create(ctx, &ufov1.CreateRequest{
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
	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, resp.GetUuid(), sighting.GetUuid())
	assert.Equal(t, "Test Location", sighting.GetLocation())
	assert.Equal(t, "Test Description", sighting.GetDescription())
	assert.Equal(t, "Green", sighting.GetColor().GetValue())
	assert.True(t, sighting.GetSound().GetValue())
	assert.Equal(t, int32(120), sighting.GetDurationSeconds().GetValue())
	assert.Equal(t, observedAt.Unix(), sighting.GetObservedAt().AsTime().Unix())
	assert.NotNil(t, sighting.GetCreatedAt(), "created_at должен быть установлен")
	assert.Nil(t, sighting.GetUpdatedAt(), "updated_at должен быть nil для новой записи")
	assert.Nil(t, sighting.GetDeletedAt(), "deleted_at должен быть nil для новой записи")
}

func TestCreate_MinimalFields(t *testing.T) {
	ctx := context.Background()

	resp, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    "Minimal Location",
		Description: "Minimal Description",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid())

	// Проверяем, что опциональные поля nil
	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	assert.Nil(t, getResp.GetSighting().GetColor())
	assert.Nil(t, getResp.GetSighting().GetSound())
	assert.Nil(t, getResp.GetSighting().GetDurationSeconds())
}

func TestCreate_UniqueUUIDs(t *testing.T) {
	ctx := context.Background()

	resp1, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    "Location 1",
		Description: "Description 1",
	})
	require.NoError(t, err)

	resp2, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    "Location 2",
		Description: "Description 2",
	})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetUuid(), resp2.GetUuid(), "UUID должны быть уникальными")
}

func TestCreate_NilInfo(t *testing.T) {
	ctx := context.Background()

	// Пустой location должен вызвать ошибку
	_, err := client.Create(ctx, &ufov1.CreateRequest{
		Location: "",
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

// TestCreate_WeatherGracefulDegradation проверяет, что сервис корректно работает
// при использовании stub-клиента погоды (graceful degradation)
// Stub-клиент возвращает захардкоженные данные, поэтому создание проходит успешно
func TestCreate_WeatherGracefulDegradation(t *testing.T) {
	ctx := context.Background()

	// Создаём наблюдение — stub-клиент погоды используется, создание должно пройти успешно
	resp, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    "Город без погоды",
		Description: "Тест graceful degradation",
	})

	require.NoError(t, err, "создание должно пройти успешно даже без сервиса погоды")
	assert.NotEmpty(t, resp.GetUuid())

	// Проверяем, что данные корректно сохранились
	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "Город без погоды", sighting.GetLocation())
	assert.Equal(t, "Тест graceful degradation", sighting.GetDescription())
}

// ============ Get Tests ============.

func TestGet_Success(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Get Test Location", "Get Test Description")

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, createdUUID, sighting.GetUuid())
	assert.Equal(t, "Get Test Location", sighting.GetLocation())
	assert.Equal(t, "Get Test Description", sighting.GetDescription())
	assert.NotNil(t, sighting.GetCreatedAt())
}

func TestGet_NotFound(t *testing.T) {
	ctx := context.Background()

	nonExistentUUID := uuid.NewString()

	_, err := client.Get(ctx, &ufov1.GetRequest{Uuid: nonExistentUUID})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGet_EmptyUUID(t *testing.T) {
	ctx := context.Background()

	_, err := client.Get(ctx, &ufov1.GetRequest{Uuid: ""})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestGet_AllFieldsPreserved(t *testing.T) {
	ctx := context.Background()

	observedAt := time.Now().Truncate(time.Microsecond)

	resp, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:      timestamppb.New(observedAt),
		Location:        "All Fields Location",
		Description:     "All Fields Description",
		Color:           wrapperspb.String("Red"),
		Sound:           wrapperspb.Bool(false),
		DurationSeconds: wrapperspb.Int32(300),
	})
	require.NoError(t, err)

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "All Fields Location", sighting.GetLocation())
	assert.Equal(t, "All Fields Description", sighting.GetDescription())
	assert.Equal(t, "Red", sighting.GetColor().GetValue())
	assert.False(t, sighting.GetSound().GetValue(), "sound=false должен сохраниться")
	assert.Equal(t, int32(300), sighting.GetDurationSeconds().GetValue())
	assert.Equal(t, observedAt.Unix(), sighting.GetObservedAt().AsTime().Unix())
}

// ============ Update Tests ============.

func TestUpdate_SingleField(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Original Location", "Original Description")

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("Updated Location"),
	})
	require.NoError(t, err)

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	assert.Equal(t, "Updated Location", getResp.GetSighting().GetLocation())
	assert.Equal(t, "Original Description", getResp.GetSighting().GetDescription())
}

func TestUpdate_MultipleFields(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Original Location", "Original Description")

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:        createdUUID,
		Location:    wrapperspb.String("Updated Location"),
		Description: wrapperspb.String("Updated Description"),
		Color:       wrapperspb.String("Blue"),
	})
	require.NoError(t, err)

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "Updated Location", sighting.GetLocation())
	assert.Equal(t, "Updated Description", sighting.GetDescription())
	assert.Equal(t, "Blue", sighting.GetColor().GetValue())
}

func TestUpdate_AllFields(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Original Location", "Original Description")

	newObservedAt := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:            createdUUID,
		ObservedAt:      timestamppb.New(newObservedAt),
		Location:        wrapperspb.String("All Updated Location"),
		Description:     wrapperspb.String("All Updated Description"),
		Color:           wrapperspb.String("Yellow"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(999),
	})
	require.NoError(t, err)

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "All Updated Location", sighting.GetLocation())
	assert.Equal(t, "All Updated Description", sighting.GetDescription())
	assert.Equal(t, "Yellow", sighting.GetColor().GetValue())
	assert.True(t, sighting.GetSound().GetValue())
	assert.Equal(t, int32(999), sighting.GetDurationSeconds().GetValue())
	assert.Equal(t, newObservedAt.Unix(), sighting.GetObservedAt().AsTime().Unix())
}

func TestUpdate_NotFound(t *testing.T) {
	ctx := context.Background()

	nonExistentUUID := uuid.NewString()

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     nonExistentUUID,
		Location: wrapperspb.String("New Location"),
	})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestUpdate_NilUpdateInfo(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Location", "Description")

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid: createdUUID,
	})

	// Пустой update без uuid-ошибок — теперь просто no-op, не ошибка
	require.NoError(t, err)
}

func TestUpdate_EmptyUUID(t *testing.T) {
	ctx := context.Background()

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     "",
		Location: wrapperspb.String("New Location"),
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestUpdate_UpdatesTimestamp(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Location", "Description")

	// Получаем исходные данные
	getResp1, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	createdAt := getResp1.GetSighting().GetCreatedAt()

	// Обновляем
	_, err = client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("New Location"),
	})
	require.NoError(t, err)

	// Проверяем состояние после обновления
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp2.GetSighting()
	assert.Nil(t, getResp1.GetSighting().GetUpdatedAt(), "updated_at должен быть nil до обновления")
	assert.NotNil(t, sighting.GetUpdatedAt(), "updated_at должен быть установлен после обновления")
	assert.Equal(t, "New Location", sighting.GetLocation(), "location должен обновиться")
	assert.Equal(t, "Description", sighting.GetDescription(), "description не должен измениться")
	assert.Equal(t, createdAt.AsTime().Unix(), sighting.GetCreatedAt().AsTime().Unix(), "created_at не должен измениться")
	assert.Nil(t, sighting.GetDeletedAt(), "deleted_at должен оставаться nil")
}

// TestUpdate_DeletedRecord проверяет, что обновление удалённой записи возвращает NotFound
// В отличие от week_1, в layers-архитектуре repository.Get() проверяет deleted_at,
// поэтому попытка обновить удалённую запись приводит к ошибке
func TestUpdate_DeletedRecord(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "To Be Deleted", "Then Updated")

	// Удаляем запись
	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Попытка обновить удалённую запись
	_, err = client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("Should Fail"),
	})

	assertGRPCStatus(t, err, codes.NotFound)
}

// ============ Delete Tests ============.

func TestDelete_Success(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "To Delete Location", "To Delete Description")

	// Проверяем, что запись существует до удаления
	_, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// После удаления Get возвращает NotFound (мягкое удаление,
	// repository.Get проверяет deleted_at)
	_, err = client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestDelete_NotFound(t *testing.T) {
	ctx := context.Background()

	nonExistentUUID := uuid.NewString()

	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: nonExistentUUID})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestDelete_EmptyUUID(t *testing.T) {
	ctx := context.Background()

	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: ""})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

// TestDelete_AlreadyDeleted проверяет идемпотентность удаления
// В layers-архитектуре repository.Delete() не проверяет deleted_at,
// поэтому повторное удаление проходит успешно (идемпотентно)
func TestDelete_AlreadyDeleted(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Double Delete", "Double Delete")

	// Первое удаление
	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Второе удаление - должно быть идемпотентным
	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err, "повторное удаление должно быть идемпотентным")
}

// ============ Full Lifecycle Test ============.

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Create
	createResp, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:  timestamppb.New(time.Now()),
		Location:    "Lifecycle Location",
		Description: "Lifecycle Description",
	})
	require.NoError(t, err)
	createdUUID := createResp.GetUuid()
	assert.NotEmpty(t, createdUUID)

	// 2. Get после создания
	getResp1, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle Location", getResp1.GetSighting().GetLocation())
	assert.NotNil(t, getResp1.GetSighting().GetCreatedAt())
	assert.Nil(t, getResp1.GetSighting().GetUpdatedAt())
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	// 3. Update
	_, err = client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:     createdUUID,
		Location: wrapperspb.String("Updated Lifecycle Location"),
		Color:    wrapperspb.String("Silver"),
	})
	require.NoError(t, err)

	// 4. Get после обновления
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, "Updated Lifecycle Location", getResp2.GetSighting().GetLocation())
	assert.Equal(t, "Silver", getResp2.GetSighting().GetColor().GetValue())
	assert.NotNil(t, getResp2.GetSighting().GetUpdatedAt())
	assert.Nil(t, getResp2.GetSighting().GetDeletedAt())

	// 5. Delete
	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// 6. Get после удаления - запись больше не доступна (мягкое удаление)
	_, err = client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	assertGRPCStatus(t, err, codes.NotFound)
}

// TestFullLifecycle_AllFields проверяет полный цикл CRUD со всеми полями
func TestFullLifecycle_AllFields(t *testing.T) {
	ctx := context.Background()

	observedAt := time.Now().Truncate(time.Microsecond)

	// 1. Create со всеми полями
	createResp, err := client.Create(ctx, &ufov1.CreateRequest{
		ObservedAt:      timestamppb.New(observedAt),
		Location:        "Full Lifecycle Location",
		Description:     "Full Lifecycle Description",
		Color:           wrapperspb.String("Blue"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(60),
	})
	require.NoError(t, err)
	createdUUID := createResp.GetUuid()

	// 2. Get — проверяем все поля
	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "Full Lifecycle Location", sighting.GetLocation())
	assert.Equal(t, "Full Lifecycle Description", sighting.GetDescription())
	assert.Equal(t, "Blue", sighting.GetColor().GetValue())
	assert.True(t, sighting.GetSound().GetValue())
	assert.Equal(t, int32(60), sighting.GetDurationSeconds().GetValue())
	assert.Equal(t, observedAt.Unix(), sighting.GetObservedAt().AsTime().Unix())

	// 3. Update — обновляем все 6 полей
	newObservedAt := time.Now().Add(-48 * time.Hour).Truncate(time.Microsecond)
	_, err = client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:            createdUUID,
		ObservedAt:      timestamppb.New(newObservedAt),
		Location:        wrapperspb.String("Updated Full Location"),
		Description:     wrapperspb.String("Updated Full Description"),
		Color:           wrapperspb.String("Red"),
		Sound:           wrapperspb.Bool(false),
		DurationSeconds: wrapperspb.Int32(999),
	})
	require.NoError(t, err)

	// 4. Get — проверяем обновлённые значения
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting2 := getResp2.GetSighting()
	assert.Equal(t, "Updated Full Location", sighting2.GetLocation())
	assert.Equal(t, "Updated Full Description", sighting2.GetDescription())
	assert.Equal(t, "Red", sighting2.GetColor().GetValue())
	assert.False(t, sighting2.GetSound().GetValue())
	assert.Equal(t, int32(999), sighting2.GetDurationSeconds().GetValue())
	assert.Equal(t, newObservedAt.Unix(), sighting2.GetObservedAt().AsTime().Unix())
	assert.NotNil(t, sighting2.GetUpdatedAt())

	// 5. Delete и проверка
	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	_, err = client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	assertGRPCStatus(t, err, codes.NotFound)
}
