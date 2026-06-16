package tracing

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// TraceIDUnaryServerInterceptor — серверный интерцептор, добавляющий trace ID
// в заголовки gRPC ответа, чтобы клиент мог найти трейс в Jaeger/Tempo
//
// Graceful degradation: если TracerProvider не инициализирован (InitTracer ещё
// не вызван или отключён в конфиге — например, в API-тестах с bufconn),
// TraceIDFromContext вернёт пустую строку, и header не будет установлен
// Это штатное поведение — сам вызов handler'а продолжится без ошибок
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
