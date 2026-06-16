// Package interceptor содержит gRPC-интерцепторы
package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sessionv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/pkg/proto/session/v1"
)

const authorizationHeader = "authorization"

// publicMethods — методы, не требующие токена авторизации
var publicMethods = map[string]struct{}{
	sessionv1.SessionService_Register_FullMethodName: {},
	sessionv1.SessionService_Login_FullMethodName:    {},
}

type ctxKey int

const tokenKey ctxKey = 0

// Auth извлекает Bearer-токен из gRPC metadata и сохраняет в контексте
// Для защищённых методов токен обязателен — без него возвращается Unauthenticated
func Auth(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	// Публичные методы пропускаем без проверки токена
	if _, ok := publicMethods[info.FullMethod]; ok {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "отсутствуют metadata")
	}

	values := md.Get(authorizationHeader)
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "отсутствует authorization header")
	}

	token, found := strings.CutPrefix(values[0], "Bearer ")
	if !found || token == "" {
		return nil, status.Error(codes.Unauthenticated, "формат: Bearer <token>")
	}

	ctx = context.WithValue(ctx, tokenKey, token)

	return handler(ctx, req)
}

// TokenFromContext возвращает токен из контекста
func TokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(tokenKey).(string)
	return token
}
