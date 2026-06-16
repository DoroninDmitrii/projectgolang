// Package interceptor содержит gRPC серверные интерцепторы
package interceptor

import (
	"context"
	"log/slog"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor создаёт серверный унарный интерцептор, который логирует
// метод, длительность и ошибку каждого gRPC вызова
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		startTime := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(startTime)

		attrs := []slog.Attr{
			slog.String("method", path.Base(info.FullMethod)),
			slog.Duration("duration", duration),
		}

		if err != nil {
			st, _ := status.FromError(err)
			attrs = append(
				attrs,
				slog.String("code", st.Code().String()),
				slog.String("error", err.Error()),
			)
			slog.LogAttrs(ctx, slog.LevelError, "gRPC метод завершён с ошибкой", attrs...)

			return resp, err
		}

		slog.LogAttrs(ctx, slog.LevelInfo, "gRPC метод завершён успешно", attrs...)

		return resp, err
	}
}
