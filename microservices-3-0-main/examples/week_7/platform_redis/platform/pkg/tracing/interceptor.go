package tracing

import (
	"context"
	"log/slog"

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
			if err := grpc.SetHeader(ctx, metadata.Pairs(TraceIDHeader, traceID)); err != nil {
				slog.WarnContext(ctx, "не удалось установить trace ID в заголовок ответа", slog.String("error", err.Error()))
			}
		}

		return handler(ctx, req)
	}
}
