package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka"
)

// KafkaProducer определяет контракт для отправки сообщений в Kafka
type KafkaProducer interface {
	Send(ctx context.Context, msg *kafka.Message) error
}
