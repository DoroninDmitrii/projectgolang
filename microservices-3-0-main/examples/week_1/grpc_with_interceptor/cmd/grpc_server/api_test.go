package main

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

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_with_interceptor/internal/interceptor"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_with_interceptor/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

var (
	lis    *bufconn.Listener
	client ufov1.UFOServiceClient
)

// TestMain запускает in-memory gRPC сервер через bufconn перед тестами
// Сервер создаётся с интерцепторами (recovery + logging), как в main.go
func TestMain(m *testing.M) {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.RecoveryInterceptor(),
			interceptor.LoggerInterceptor(),
		),
	)

	service := &ufoService{
		sightings: make(map[string]*ufov1.Sighting),
	}
	ufov1.RegisterUFOServiceServer(s, service)

	go func() {
		if serveErr := s.Serve(lis); serveErr != nil {
			panic(serveErr)
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

	require.Error(t, err, "ожидалась ошибка")

	st, ok := status.FromError(err)
	assert.True(t, ok, "ошибка должна быть gRPC статусом")
	assert.Equal(t, expectedCode, st.Code(), "неверный gRPC код: %s", st.Message())
}

// createTestSighting создаёт тестовую запись и возвращает её UUID
func createTestSighting(t *testing.T, client ufov1.UFOServiceClient, location, description string) string {
	t.Helper()

	resp, err := client.Create(context.Background(), &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    location,
			Description: description,
		},
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
		Info: &ufov1.SightingInfo{
			ObservedAt:      timestamppb.New(observedAt),
			Location:        "Test Location",
			Description:     "Test Description",
			Color:           wrapperspb.String("Green"),
			Sound:           wrapperspb.Bool(true),
			DurationSeconds: wrapperspb.Int32(120),
		},
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
	assert.Equal(t, "Test Location", sighting.GetInfo().GetLocation())
	assert.Equal(t, "Test Description", sighting.GetInfo().GetDescription())
	assert.Equal(t, "Green", sighting.GetInfo().GetColor().GetValue())
	assert.True(t, sighting.GetInfo().GetSound().GetValue())
	assert.Equal(t, int32(120), sighting.GetInfo().GetDurationSeconds().GetValue())
	assert.Equal(t, observedAt.Unix(), sighting.GetInfo().GetObservedAt().AsTime().Unix())
	assert.NotNil(t, sighting.GetCreatedAt(), "created_at должен быть установлен")
	assert.Nil(t, sighting.GetUpdatedAt(), "updated_at должен быть nil для новой записи")
	assert.Nil(t, sighting.GetDeletedAt(), "deleted_at должен быть nil для новой записи")
}

func TestCreate_MinimalFields(t *testing.T) {
	ctx := context.Background()

	resp, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    "Minimal Location",
			Description: "Minimal Description",
		},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid())

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)

	assert.Nil(t, getResp.GetSighting().GetInfo().GetColor())
	assert.Nil(t, getResp.GetSighting().GetInfo().GetSound())
	assert.Nil(t, getResp.GetSighting().GetInfo().GetDurationSeconds())
}

func TestCreate_UniqueUUIDs(t *testing.T) {
	ctx := context.Background()

	resp1, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    "Location 1",
			Description: "Description 1",
		},
	})
	require.NoError(t, err)

	resp2, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    "Location 2",
			Description: "Description 2",
		},
	})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetUuid(), resp2.GetUuid(), "UUID должны быть уникальными")
}

func TestCreate_NilInfo(t *testing.T) {
	ctx := context.Background()

	_, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: nil,
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

// ============ Get Tests ============.

func TestGet_Success(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Get Test Location", "Get Test Description")

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, createdUUID, sighting.GetUuid())
	assert.Equal(t, "Get Test Location", sighting.GetInfo().GetLocation())
	assert.Equal(t, "Get Test Description", sighting.GetInfo().GetDescription())
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

// ============ Update Tests ============.

func TestUpdate_SingleField(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Original Location", "Original Description")

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid: createdUUID,
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("Updated Location"),
		},
	})
	require.NoError(t, err)

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	assert.Equal(t, "Updated Location", getResp.GetSighting().GetInfo().GetLocation())
	assert.Equal(t, "Original Description", getResp.GetSighting().GetInfo().GetDescription())
}

func TestUpdate_MultipleFields(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Original Location", "Original Description")

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid: createdUUID,
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location:    wrapperspb.String("Updated Location"),
			Description: wrapperspb.String("Updated Description"),
			Color:       wrapperspb.String("Blue"),
		},
	})
	require.NoError(t, err)

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, "Updated Location", sighting.GetInfo().GetLocation())
	assert.Equal(t, "Updated Description", sighting.GetInfo().GetDescription())
	assert.Equal(t, "Blue", sighting.GetInfo().GetColor().GetValue())
}

func TestUpdate_NotFound(t *testing.T) {
	ctx := context.Background()

	nonExistentUUID := uuid.NewString()

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid: nonExistentUUID,
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("New Location"),
		},
	})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestUpdate_NilUpdateInfo(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Location", "Description")

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:       createdUUID,
		UpdateInfo: nil,
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestUpdate_EmptyUUID(t *testing.T) {
	ctx := context.Background()

	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid: "",
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("New Location"),
		},
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
		Uuid: createdUUID,
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("New Location"),
		},
	})
	require.NoError(t, err)

	// Проверяем состояние после обновления
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp2.GetSighting()
	assert.Nil(t, getResp1.GetSighting().GetUpdatedAt(), "updated_at должен быть nil до обновления")
	assert.NotNil(t, sighting.GetUpdatedAt(), "updated_at должен быть установлен после обновления")
	assert.NotEqual(t, sighting.GetCreatedAt().AsTime(), sighting.GetUpdatedAt().AsTime(), "updated_at должен отличаться от created_at")
	assert.Equal(t, "New Location", sighting.GetInfo().GetLocation(), "location должен обновиться")
	assert.Equal(t, "Description", sighting.GetInfo().GetDescription(), "description не должен измениться")
	assert.Equal(t, createdAt.AsTime().Unix(), sighting.GetCreatedAt().AsTime().Unix(), "created_at не должен измениться")
	assert.Nil(t, sighting.GetDeletedAt(), "deleted_at должен оставаться nil")
}

// ============ Delete Tests ============.

func TestDelete_Success(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "To Delete Location", "To Delete Description")

	// Проверяем состояние до удаления
	getResp1, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt(), "deleted_at должен быть nil до удаления")

	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Проверяем состояние после удаления (мягкое удаление — Get всё ещё работает)
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)

	sighting := getResp2.GetSighting()
	assert.NotNil(t, sighting.GetDeletedAt(), "deleted_at должен быть установлен")
	assert.Equal(t, "To Delete Location", sighting.GetInfo().GetLocation(), "данные должны сохраниться")
	assert.Equal(t, "To Delete Description", sighting.GetInfo().GetDescription(), "данные должны сохраниться")
	assert.NotNil(t, sighting.GetCreatedAt(), "created_at должен остаться")
}

func TestDelete_NotFound(t *testing.T) {
	ctx := context.Background()

	nonExistentUUID := uuid.NewString()

	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: nonExistentUUID})

	assertGRPCStatus(t, err, codes.NotFound)
}

func TestDelete_AlreadyDeleted(t *testing.T) {
	ctx := context.Background()

	createdUUID := createTestSighting(t, client, "Double Delete", "Double Delete")

	// Первое удаление
	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// Проверяем, что deleted_at установлен после первого удаления
	getResp1, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotNil(t, getResp1.GetSighting().GetDeletedAt(), "deleted_at должен быть установлен после первого удаления")

	// Второе удаление - должно быть идемпотентным
	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err, "повторное удаление должно быть идемпотентным")

	// Проверяем, что deleted_at остался установлен
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotNil(t, getResp2.GetSighting().GetDeletedAt(), "deleted_at должен остаться установлен")
}

func TestDelete_EmptyUUID(t *testing.T) {
	ctx := context.Background()

	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: ""})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

// ============ Full Lifecycle Test ============.

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Create
	createResp, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    "Lifecycle Location",
			Description: "Lifecycle Description",
		},
	})
	require.NoError(t, err)
	createdUUID := createResp.GetUuid()
	assert.NotEmpty(t, createdUUID)

	// 2. Get после создания
	getResp1, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle Location", getResp1.GetSighting().GetInfo().GetLocation())
	assert.NotNil(t, getResp1.GetSighting().GetCreatedAt())
	assert.Nil(t, getResp1.GetSighting().GetUpdatedAt())
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	// 3. Update
	_, err = client.Update(ctx, &ufov1.UpdateRequest{
		Uuid: createdUUID,
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("Updated Lifecycle Location"),
		},
	})
	require.NoError(t, err)

	// 4. Get после обновления
	getResp2, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.Equal(t, "Updated Lifecycle Location", getResp2.GetSighting().GetInfo().GetLocation())
	assert.NotNil(t, getResp2.GetSighting().GetUpdatedAt())
	assert.Nil(t, getResp2.GetSighting().GetDeletedAt())

	// 5. Delete
	_, err = client.Delete(ctx, &ufov1.DeleteRequest{Uuid: createdUUID})
	require.NoError(t, err)

	// 6. Get после удаления — мягкое удаление, запись доступна с deleted_at
	getResp3, err := client.Get(ctx, &ufov1.GetRequest{Uuid: createdUUID})
	require.NoError(t, err)
	assert.NotNil(t, getResp3.GetSighting().GetDeletedAt(), "запись должна быть помечена как удалённая")
}
