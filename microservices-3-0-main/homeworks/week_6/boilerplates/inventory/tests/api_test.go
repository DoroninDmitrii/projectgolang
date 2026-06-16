//go:build apitest

// Package tests содержит API-тесты gRPC auth-interceptor InventoryService
//
// Использует bufconn для мока IAM AuthService и для тестируемого Inventory gRPC-сервера —
// Whoami возвращает контролируемый ответ, а Inventory-метод возвращает текущий userUUID
// из контекста, чтобы тест мог проверить, что interceptor его прокинул
package tests

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	iamClient "github.com/student/inventory/internal/client/grpc/iam/v1"
	"github.com/student/inventory/internal/interceptor"
	"github.com/student/platform/pkg/auth"
	authv1 "github.com/student/shared/pkg/proto/auth/v1"
	commonv1 "github.com/student/shared/pkg/proto/common/v1"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
)

const bufSize = 1024 * 1024

type stubAuthServer struct {
	authv1.UnimplementedAuthServiceServer
	whoami func(string) (*authv1.WhoamiResponse, error)
}

func (s *stubAuthServer) Whoami(_ context.Context, req *authv1.WhoamiRequest) (*authv1.WhoamiResponse, error) {
	return s.whoami(req.GetSessionUuid())
}

// stubInventoryServer возвращает userUUID из ctx в имени детали — чтобы тест мог
// убедиться, что interceptor положил его в контекст и хендлер увидел
type stubInventoryServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
}

func (s *stubInventoryServer) GetPart(ctx context.Context, _ *inventoryv1.GetPartRequest) (*inventoryv1.GetPartResponse, error) {
	uid, _ := auth.UserUUIDFromContext(ctx)
	return &inventoryv1.GetPartResponse{
		Part: &inventoryv1.Part{
			Uuid: "00000000-0000-0000-0000-000000000000",
			Name: uid.String(),
		},
	}, nil
}

func startStubAuth(t *testing.T, stub *stubAuthServer) *iamClient.Client {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	authv1.RegisterAuthServiceServer(server, stub)

	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return iamClient.New(authv1.NewAuthServiceClient(conn))
}

func startInventory(t *testing.T, authClient *iamClient.Client) inventoryv1.InventoryServiceClient {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor.Auth(authClient)),
	)
	inventoryv1.RegisterInventoryServiceServer(server, &stubInventoryServer{})

	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return inventoryv1.NewInventoryServiceClient(conn)
}

func TestInterceptor_NoMetadata(t *testing.T) {
	authClient := startStubAuth(t, &stubAuthServer{
		whoami: func(string) (*authv1.WhoamiResponse, error) {
			t.Fatal("Whoami не должен вызываться без metadata")
			return nil, nil
		},
	})
	invClient := startInventory(t, authClient)

	_, err := invClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: "x"})
	requireGRPCCode(t, err, codes.Unauthenticated)
}

func TestInterceptor_EmptySession(t *testing.T) {
	authClient := startStubAuth(t, &stubAuthServer{
		whoami: func(string) (*authv1.WhoamiResponse, error) {
			t.Fatal("Whoami не должен вызываться при пустом session-uuid")
			return nil, nil
		},
	})
	invClient := startInventory(t, authClient)

	ctx := metadata.AppendToOutgoingContext(context.Background(), interceptor.SessionMetadataKey, "")

	_, err := invClient.GetPart(ctx, &inventoryv1.GetPartRequest{Uuid: "x"})
	requireGRPCCode(t, err, codes.Unauthenticated)
}

func TestInterceptor_InvalidSession(t *testing.T) {
	authClient := startStubAuth(t, &stubAuthServer{
		whoami: func(string) (*authv1.WhoamiResponse, error) {
			return nil, status.Error(codes.Unauthenticated, "сессия не найдена")
		},
	})
	invClient := startInventory(t, authClient)

	ctx := metadata.AppendToOutgoingContext(context.Background(), interceptor.SessionMetadataKey, "expired")

	_, err := invClient.GetPart(ctx, &inventoryv1.GetPartRequest{Uuid: "x"})
	requireGRPCCode(t, err, codes.Unauthenticated)
}

func TestInterceptor_ValidSession(t *testing.T) {
	const (
		userUUID    = "11111111-1111-1111-1111-111111111111"
		sessionUUID = "22222222-2222-2222-2222-222222222222"
	)
	authClient := startStubAuth(t, &stubAuthServer{
		whoami: func(s string) (*authv1.WhoamiResponse, error) {
			require.Equal(t, sessionUUID, s)
			return &authv1.WhoamiResponse{
				Session: &commonv1.Session{Uuid: s, CreatedAt: timestamppb.Now(), ExpiresAt: timestamppb.Now()},
				User:    &commonv1.User{Uuid: userUUID, Info: &commonv1.UserInfo{Login: "alice"}},
			}, nil
		},
	})
	invClient := startInventory(t, authClient)

	ctx := metadata.AppendToOutgoingContext(context.Background(), interceptor.SessionMetadataKey, sessionUUID)

	resp, err := invClient.GetPart(ctx, &inventoryv1.GetPartRequest{Uuid: "x"})
	require.NoError(t, err)
	require.Equal(t, userUUID, resp.GetPart().GetName())
}

func requireGRPCCode(t *testing.T, err error, expected codes.Code) {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "ожидалась gRPC-ошибка, получили: %v", err)
	require.Equal(t, expected, st.Code(), "сообщение: %s", st.Message())
}
