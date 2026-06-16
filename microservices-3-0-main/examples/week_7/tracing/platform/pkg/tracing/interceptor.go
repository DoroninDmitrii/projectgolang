package tracing

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// TraceIDUnaryServerInterceptor — серверный интерцептор, добавляющий trace ID
// в заголовки gRPC ответа, чтобы клиент мог найти трейс в Jaeger/Tempo
func TraceIDUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		traceID := TraceIDFromContext(ctx)
		if traceID != "" {
			// grpc.SetHeader добавляет метаданные в заголовки gRPC ответа,
			// которые клиент получит вместе с результатом вызова
			_ = grpc.SetHeader(ctx, metadata.Pairs(TraceIDHeader, traceID)) //nolint:gosec // ошибка установки заголовка не критична — trace ID опционален
		}

		return handler(ctx, req)
	}
}
