// Package interceptor содержит перехватчики gRPC-запросов PaymentService.
package interceptor

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error преобразует необработанные ошибки в gRPC-статус.
func Error(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	resp, err := handler(ctx, req)
	if err == nil {
		return resp, nil
	}

	if _, ok := status.FromError(err); ok {
		return resp, err
	}

	slog.ErrorContext(ctx, "необработанная ошибка",
		"method", info.FullMethod,
		"error", err,
	)

	return nil, status.Error(codes.Internal, "внутренняя ошибка")
}
