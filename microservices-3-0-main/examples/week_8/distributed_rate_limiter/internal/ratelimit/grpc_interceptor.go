package ratelimit

import (
	"context"
	"log/slog"

	"github.com/go-redis/redis_rate/v10"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewGRPCInterceptor создаёт gRPC unary server interceptor с распределённым rate limiter
//
// Библиотека redis_rate использует алгоритм GCRA (Generic Cell Rate Algorithm) —
// это вариант token bucket, реализованный атомарно в Redis через Lua-скрипт
// Благодаря Redis все инстансы сервиса разделяют общий счётчик запросов,
// поэтому лимит работает глобально, а не per-instance
//
// methodLimits задаёт лимиты для конкретных gRPC-методов (ключ — info.FullMethod)
// defaultLimit применяется к методам, не указанным в methodLimits
func NewGRPCInterceptor(
	limiter *redis_rate.Limiter,
	defaultLimit redis_rate.Limit,
	methodLimits map[string]redis_rate.Limit,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		limit := defaultLimit
		if ml, ok := methodLimits[info.FullMethod]; ok {
			limit = ml
		}

		// Библиотека redis_rate сама добавляет префикс "rate:" к ключу (константа redisPrefix),
		// поэтому дополнительный префикс не нужен
		key := info.FullMethod

		res, err := limiter.Allow(ctx, key, limit)
		if err != nil {
			slog.Error("ошибка проверки rate limit", "method", info.FullMethod, "error", err)
			return handler(ctx, req)
		}

		if res.Allowed == 0 {
			slog.Warn(
				"запрос отклонён rate limiter",
				"method", info.FullMethod,
				"retry_after", res.RetryAfter,
			)

			return nil, status.Error(codes.ResourceExhausted, "слишком много запросов, попробуйте позже")
		}

		return handler(ctx, req)
	}
}
