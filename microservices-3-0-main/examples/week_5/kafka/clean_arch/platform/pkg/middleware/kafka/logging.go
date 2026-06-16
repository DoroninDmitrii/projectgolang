package kafka

import (
	"context"
	"log/slog"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka"
)

// ConsumerLogging — middleware для логирования входящих сообщений
func ConsumerLogging() kafka.Middleware {
	return func(next kafka.MessageHandler) kafka.MessageHandler {
		return func(ctx context.Context, msg kafka.Message) error {
			slog.InfoContext(ctx, "получено сообщение Kafka", "topic", msg.Topic)
			return next(ctx, msg)
		}
	}
}
