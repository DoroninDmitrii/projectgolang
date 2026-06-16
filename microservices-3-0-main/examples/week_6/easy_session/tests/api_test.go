// Package tests содержит интеграционные тесты для SessionService
package tests

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/internal/interceptor"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/internal/server"
	sessionv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/pkg/proto/session/v1"
)

const bufSize = 1024 * 1024

var client sessionv1.SessionServiceClient

func TestMain(m *testing.M) {
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.Auth),
	)
	sessionv1.RegisterSessionServiceServer(grpcServer, server.NewServer())

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(fmt.Sprintf("ошибка gRPC сервера: %v", err))
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка подключения: %v\n", err)
		os.Exit(1)
	}

	client = sessionv1.NewSessionServiceClient(conn)

	code := m.Run()

	conn.Close()
	grpcServer.GracefulStop()

	os.Exit(code)
}

// withToken добавляет Bearer-токен в исходящий контекст gRPC
func withToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func TestRegister_Success(t *testing.T) {
	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
	})
	require.NoError(t, err)
}

func TestRegister_DuplicateEmail(t *testing.T) {
	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    "duplicate@example.com",
		Password: "password123",
		Name:     "User 1",
	})
	require.NoError(t, err)

	_, err = client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    "duplicate@example.com",
		Password: "password456",
		Name:     "User 2",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

func TestRegister_InvalidEmail(t *testing.T) {
	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    "invalid-email",
		Password: "password123",
		Name:     "Test",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestRegister_EmptyFields(t *testing.T) {
	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    "",
		Password: "",
		Name:     "",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestLogin_Success(t *testing.T) {
	email := "login-test@example.com"

	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    email,
		Password: "mypassword",
		Name:     "Login Test",
	})
	require.NoError(t, err)

	resp, err := client.Login(context.Background(), &sessionv1.LoginRequest{
		Email:    email,
		Password: "mypassword",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetSessionToken())
}

func TestLogin_WrongPassword(t *testing.T) {
	email := "wrong-pass@example.com"

	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    email,
		Password: "correctpassword",
		Name:     "Wrong Pass Test",
	})
	require.NoError(t, err)

	_, err = client.Login(context.Background(), &sessionv1.LoginRequest{
		Email:    email,
		Password: "wrongpassword",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestLogin_NonExistentUser(t *testing.T) {
	_, err := client.Login(context.Background(), &sessionv1.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "password",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestWhoami_Success(t *testing.T) {
	email := "whoami@example.com"
	name := "Whoami User"

	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    email,
		Password: "pass123",
		Name:     name,
	})
	require.NoError(t, err)

	loginResp, err := client.Login(context.Background(), &sessionv1.LoginRequest{
		Email:    email,
		Password: "pass123",
	})
	require.NoError(t, err)

	ctx := withToken(context.Background(), loginResp.GetSessionToken())
	whoamiResp, err := client.Whoami(ctx, &sessionv1.WhoamiRequest{})
	require.NoError(t, err)
	assert.Equal(t, email, whoamiResp.GetEmail())
	assert.Equal(t, name, whoamiResp.GetName())
}

func TestWhoami_NoToken(t *testing.T) {
	_, err := client.Whoami(context.Background(), &sessionv1.WhoamiRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestWhoami_InvalidToken(t *testing.T) {
	ctx := withToken(context.Background(), "invalid-token")
	_, err := client.Whoami(ctx, &sessionv1.WhoamiRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestLogout_Success(t *testing.T) {
	email := "logout@example.com"

	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    email,
		Password: "pass123",
		Name:     "Logout User",
	})
	require.NoError(t, err)

	loginResp, err := client.Login(context.Background(), &sessionv1.LoginRequest{
		Email:    email,
		Password: "pass123",
	})
	require.NoError(t, err)

	token := loginResp.GetSessionToken()
	ctx := withToken(context.Background(), token)

	// Whoami работает до logout
	_, err = client.Whoami(ctx, &sessionv1.WhoamiRequest{})
	require.NoError(t, err)

	// Logout
	_, err = client.Logout(ctx, &sessionv1.LogoutRequest{})
	require.NoError(t, err)

	// Whoami больше не работает после logout
	_, err = client.Whoami(ctx, &sessionv1.WhoamiRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestLogout_Idempotent(t *testing.T) {
	email := "logout-idem@example.com"

	_, err := client.Register(context.Background(), &sessionv1.RegisterRequest{
		Email:    email,
		Password: "pass123",
		Name:     "Idempotent Logout",
	})
	require.NoError(t, err)

	loginResp, err := client.Login(context.Background(), &sessionv1.LoginRequest{
		Email:    email,
		Password: "pass123",
	})
	require.NoError(t, err)

	ctx := withToken(context.Background(), loginResp.GetSessionToken())

	// Первый logout
	_, err = client.Logout(ctx, &sessionv1.LogoutRequest{})
	require.NoError(t, err)

	// Второй logout (идемпотентный)
	_, err = client.Logout(ctx, &sessionv1.LogoutRequest{})
	require.NoError(t, err)
}

func TestFullFlow(t *testing.T) {
	ctx := context.Background()
	email := "fullflow@example.com"
	name := "Full Flow User"

	// 1. Register
	_, err := client.Register(ctx, &sessionv1.RegisterRequest{
		Email:    email,
		Password: "securepass",
		Name:     name,
	})
	require.NoError(t, err)

	// 2. Login
	loginResp, err := client.Login(ctx, &sessionv1.LoginRequest{
		Email:    email,
		Password: "securepass",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, loginResp.GetSessionToken())

	// 3. Whoami
	authCtx := withToken(ctx, loginResp.GetSessionToken())
	whoamiResp, err := client.Whoami(authCtx, &sessionv1.WhoamiRequest{})
	require.NoError(t, err)
	assert.Equal(t, email, whoamiResp.GetEmail())
	assert.Equal(t, name, whoamiResp.GetName())

	// 4. Logout
	_, err = client.Logout(authCtx, &sessionv1.LogoutRequest{})
	require.NoError(t, err)

	// 5. Whoami после logout — ошибка
	_, err = client.Whoami(authCtx, &sessionv1.WhoamiRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}
