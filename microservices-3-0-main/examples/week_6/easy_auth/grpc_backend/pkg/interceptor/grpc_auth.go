package interceptor

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/grpc_backend/pkg/session"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/auth"
)

// publicMethods — методы gRPC Reflection API, не требующие аутентификации
// Reflection используется инструментами вроде grpcurl и grpcui для интроспекции сервера
// (получение списка сервисов, описания методов и т.д.)
// Указаны обе версии (v1 и v1alpha), т.к. разные клиенты используют разные версии API
var publicMethods = map[string]bool{
	"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      true,
	"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": true,
}

// GRPCAuth — gRPC unary server interceptor для аутентификации
// Извлекает session-token из incoming metadata, проверяет сессию
// и добавляет пользователя в контекст
//
// gRPC metadata — аналог HTTP-заголовков, бывает двух видов:
//   - Incoming metadata — то, что пришло от клиента (на стороне сервера)
//     Читается через metadata.FromIncomingContext(ctx)
//     Используется для чтения токенов, заголовков авторизации, request-id и т.д
//   - Outgoing metadata — то, что клиент отправляет серверу (на стороне клиента)
//     Устанавливается через metadata.NewOutgoingContext(ctx, md)
//     Используется для передачи токенов, трейс-контекста и прочих заголовков при вызове
//
// Разделение нужно, чтобы сервер случайно не переслал чужие заголовки дальше по цепочке
// вызовов — incoming и outgoing живут в разных «слотах» контекста
//
// Пример: Client → ServiceA → ServiceB
// ServiceA получает request-id через incoming metadata от клиента
// Чтобы пробросить его дальше в ServiceB, нужно явно переложить значение
// из incoming в outgoing:
//
//	md, _ := metadata.FromIncomingContext(ctx)
//	outCtx := metadata.NewOutgoingContext(ctx, md)
//	serviceB.Call(outCtx, req)
//
// Если этого не сделать, ServiceB не увидит request-id — incoming автоматически
// не попадает в outgoing. Это сделано намеренно, чтобы не пробрасывать
// всё подряд (например, auth-токен клиента туда, куда он не должен попасть)
func GRPCAuth(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	// Пропускаем аутентификацию для публичных методов
	if publicMethods[info.FullMethod] {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "отсутствуют metadata")
	}

	tokens := md.Get(auth.SessionTokenKey)
	if len(tokens) == 0 {
		return nil, status.Error(codes.Unauthenticated, "отсутствует session-token в metadata")
	}

	token := tokens[0]
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "пустой session-token")
	}

	user, exists := session.Store[token]
	if !exists {
		return nil, status.Error(codes.Unauthenticated, "недействительный session-token")
	}

	slog.Info(
		"gRPC: пользователь аутентифицирован",
		"method", info.FullMethod,
		"email", user.Email,
		"role", user.Role,
	)

	ctx = auth.WithUser(ctx, user)

	return handler(ctx, req)
}
