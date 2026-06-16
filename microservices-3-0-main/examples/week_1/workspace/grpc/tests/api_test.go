package tests

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ufoService "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/grpc/pkg/service"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/shared/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

var (
	lis        *bufconn.Listener
	grpcClient ufov1.UFOServiceClient
)

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

// randomSightingInfo создаёт SightingInfo с случайными данными
func randomSightingInfo() *ufov1.SightingInfo {
	return &ufov1.SightingInfo{
		ObservedAt:  timestamppb.Now(),
		Location:    fmt.Sprintf("Test Location %d", rand.IntN(10000)),
		Description: fmt.Sprintf("Test Description %d", rand.IntN(10000)),
	}
}

// TestMain запускает in-memory gRPC сервер через bufconn перед тестами
func TestMain(m *testing.M) {
	// DI инлайн: service
	lis = bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	ufov1.RegisterUFOServiceServer(grpcServer, ufoService.New())

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			panic(serveErr)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}
	grpcClient = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	conn.Close()
	grpcServer.Stop()
	os.Exit(code)
}

// Create Tests

func TestGRPC_Create_Success(t *testing.T) {
	info := randomSightingInfo()

	resp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{
		Info: info,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())

	getResp, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: resp.GetUuid(),
	})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, resp.GetUuid(), sighting.GetUuid())
	assert.Equal(t, info.GetLocation(), sighting.GetInfo().GetLocation())
	assert.Equal(t, info.GetDescription(), sighting.GetInfo().GetDescription())
	assert.NotNil(t, sighting.GetCreatedAt())
}

func TestGRPC_Create_MinimalFields(t *testing.T) {
	info := &ufov1.SightingInfo{
		Location:    "Minimal Location",
		Description: "Minimal Description",
	}

	resp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{
		Info: info,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetUuid())
}

func TestGRPC_Create_UniqueUUIDs(t *testing.T) {
	info1 := randomSightingInfo()
	info2 := randomSightingInfo()

	resp1, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info1})
	require.NoError(t, err)

	resp2, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info2})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetUuid(), resp2.GetUuid())
}

// Get Tests

func TestGRPC_Get_Success(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	getResp, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	sighting := getResp.GetSighting()
	assert.Equal(t, createResp.GetUuid(), sighting.GetUuid())
	assert.Equal(t, info.GetLocation(), sighting.GetInfo().GetLocation())
	assert.Equal(t, info.GetDescription(), sighting.GetInfo().GetDescription())
	assert.NotNil(t, sighting.GetCreatedAt())
	assert.Nil(t, sighting.GetDeletedAt())
}

func TestGRPC_Get_NotFound(t *testing.T) {
	_, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: "non-existent-uuid-12345",
	})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGRPC_Get_EmptyUUID(t *testing.T) {
	_, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: "",
	})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.NotFound)
}

// Update Tests

func TestGRPC_Update_SingleField(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	newLocation := "Updated Location"
	_, err = grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: createResp.GetUuid(),
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String(newLocation),
		},
	})
	require.NoError(t, err)

	getResp, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	assert.Equal(t, newLocation, getResp.GetSighting().GetInfo().GetLocation())
	assert.Equal(t, info.GetDescription(), getResp.GetSighting().GetInfo().GetDescription())
}

func TestGRPC_Update_MultipleFields(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	newLocation := "Multi Update Location"
	newDescription := "Multi Update Description"
	_, err = grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: createResp.GetUuid(),
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location:    wrapperspb.String(newLocation),
			Description: wrapperspb.String(newDescription),
		},
	})
	require.NoError(t, err)

	getResp, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	assert.Equal(t, newLocation, getResp.GetSighting().GetInfo().GetLocation())
	assert.Equal(t, newDescription, getResp.GetSighting().GetInfo().GetDescription())
}

func TestGRPC_Update_NotFound(t *testing.T) {
	_, err := grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: "non-existent-uuid-12345",
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("Test"),
		},
	})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGRPC_Update_NilUpdateInfo(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	_, err = grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid:       createResp.GetUuid(),
		UpdateInfo: nil,
	})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestGRPC_Update_EmptyUUID(t *testing.T) {
	_, err := grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: "",
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String("New Location"),
		},
	})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGRPC_Update_UpdatesTimestamp(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	getResp1, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)
	createdAt := getResp1.GetSighting().GetCreatedAt()

	newLocation := "Timestamp Test Location"
	_, err = grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: createResp.GetUuid(),
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String(newLocation),
		},
	})
	require.NoError(t, err)

	getResp2, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	assert.Equal(t, newLocation, getResp2.GetSighting().GetInfo().GetLocation())
	assert.Equal(t, createdAt.AsTime(), getResp2.GetSighting().GetCreatedAt().AsTime())
	assert.NotNil(t, getResp2.GetSighting().GetUpdatedAt())
	assert.True(t, getResp2.GetSighting().GetUpdatedAt().AsTime().After(createdAt.AsTime()))
}

// Delete Tests

func TestGRPC_Delete_Success(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	getResp1, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	_, err = grpcClient.Delete(context.Background(), &ufov1.DeleteRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	getResp2, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)
	assert.NotNil(t, getResp2.GetSighting().GetDeletedAt())
}

func TestGRPC_Delete_NotFound(t *testing.T) {
	_, err := grpcClient.Delete(context.Background(), &ufov1.DeleteRequest{
		Uuid: "non-existent-uuid-12345",
	})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.NotFound)
}

func TestGRPC_Delete_AlreadyDeleted(t *testing.T) {
	info := randomSightingInfo()
	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)

	_, err = grpcClient.Delete(context.Background(), &ufov1.DeleteRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	getResp1, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)
	require.NotNil(t, getResp1.GetSighting().GetDeletedAt())

	_, err = grpcClient.Delete(context.Background(), &ufov1.DeleteRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)

	getResp2, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{
		Uuid: createResp.GetUuid(),
	})
	require.NoError(t, err)
	assert.NotNil(t, getResp2.GetSighting().GetDeletedAt())
}

func TestGRPC_Delete_EmptyUUID(t *testing.T) {
	_, err := grpcClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: ""})
	require.Error(t, err)
	assertGRPCStatus(t, err, codes.NotFound)
}

// Full Lifecycle Test

func TestGRPC_FullLifecycle(t *testing.T) {
	// 1. Create
	info := &ufov1.SightingInfo{
		ObservedAt:      timestamppb.Now(),
		Location:        "Lifecycle Test Location",
		Description:     "Lifecycle Test Description",
		Color:           wrapperspb.String("blue"),
		Sound:           wrapperspb.Bool(true),
		DurationSeconds: wrapperspb.Int32(120),
	}

	createResp, err := grpcClient.Create(context.Background(), &ufov1.CreateRequest{Info: info})
	require.NoError(t, err)
	uuid := createResp.GetUuid()

	// 2. Get - проверяем созданные данные
	getResp1, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid})
	require.NoError(t, err)
	assert.Equal(t, info.GetLocation(), getResp1.GetSighting().GetInfo().GetLocation())
	assert.NotNil(t, getResp1.GetSighting().GetCreatedAt())
	assert.Nil(t, getResp1.GetSighting().GetDeletedAt())

	// 3. Update - изменяем данные
	newLocation := "Updated Lifecycle Location"
	_, err = grpcClient.Update(context.Background(), &ufov1.UpdateRequest{
		Uuid: uuid,
		UpdateInfo: &ufov1.SightingUpdateInfo{
			Location: wrapperspb.String(newLocation),
		},
	})
	require.NoError(t, err)

	// 4. Get - проверяем обновление
	getResp2, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid})
	require.NoError(t, err)
	assert.Equal(t, newLocation, getResp2.GetSighting().GetInfo().GetLocation())
	assert.NotNil(t, getResp2.GetSighting().GetUpdatedAt())

	// 5. Delete - мягкое удаление
	_, err = grpcClient.Delete(context.Background(), &ufov1.DeleteRequest{Uuid: uuid})
	require.NoError(t, err)

	// 6. Get - проверяем, что deleted_at установлен
	getResp3, err := grpcClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid})
	require.NoError(t, err)
	assert.NotNil(t, getResp3.GetSighting().GetDeletedAt())
}
