// Package interceptor содержит gRPC interceptor'ы
package interceptor

import (
	"context"
	"path"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/metrics"
)

// MetricsInterceptor записывает длительность каждого gRPC вызова в Histogram-метрику
// Атрибут rpc.method позволяет фильтровать в Prometheus по конкретному методу:
//
//	ufo_service_rpc_duration_seconds_bucket{rpc_method="Create", le="0.1"}
func MetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		startTime := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(startTime).Seconds()

		// Record записывает значение в гистограмму
		// SDK автоматически раскладывает его по бакетам и обновляет count/sum
		metrics.RPCDuration.Record(
			ctx, duration,
			metric.WithAttributes(
				attribute.String("rpc.method", path.Base(info.FullMethod)),
			),
		)

		return resp, err
	}
}
