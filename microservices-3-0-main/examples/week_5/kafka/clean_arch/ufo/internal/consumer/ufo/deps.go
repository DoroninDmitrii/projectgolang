package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka"
)

// Consumer определяет контракт для потребления сообщений из Kafka
type Consumer interface {
	Consume(ctx context.Context, handler kafka.MessageHandler) error
}
