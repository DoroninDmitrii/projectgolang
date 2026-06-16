package consumer

import (
	"context"
	"errors"
	"log/slog"

	"github.com/IBM/sarama"

	"github.com/student/platform/pkg/kafka"
)

// Option — функциональная опция для конфигурации Consumer
type Option func(*Consumer)

// WithMiddlewares добавляет middleware-цепочку к Consumer
// Middleware применяются в порядке передачи: первый оборачивает всю цепочку
func WithMiddlewares(mws ...kafka.Middleware) Option {
	return func(c *Consumer) {
		c.middlewares = append(c.middlewares, mws...)
	}
}

// Consumer — обёртка над sarama.ConsumerGroup с поддержкой middleware
//
// Запускает бесконечный цикл потребления сообщений из указанных топиков
// При ребалансировке consumer group sarama вызывает Consume повторно —
// цикл в методе Consume обрабатывает это автоматически
type Consumer struct {
	group       sarama.ConsumerGroup
	topics      []string
	middlewares []kafka.Middleware
}

// NewConsumer создаёт Consumer для указанных топиков
//
// group должен быть создан через sarama.NewConsumerGroup с корректной конфигурацией:
//   - Consumer.Offsets.Initial — стратегия начального оффсета (OffsetOldest / OffsetNewest)
//   - Consumer.Group.Rebalance.GroupStrategies — стратегия ребалансировки (RoundRobin, Range и т.д.)
func NewConsumer(group sarama.ConsumerGroup, topics []string, opts ...Option) *Consumer {
	c := &Consumer{
		group:  group,
		topics: topics,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Consume запускает бесконечный цикл потребления сообщений
//
// handler вызывается для каждого сообщения. При успешном возврате (nil) сообщение
// помечается как обработанное (at-least-once семантика). При ошибке — сообщение
// логируется и пропускается (оффсет НЕ коммитится, но обработка продолжается)
//
// Метод блокирует горутину до отмены ctx или критической ошибки
func (c *Consumer) Consume(ctx context.Context, handler kafka.MessageHandler) error {
	newGroupHandler := NewGroupHandler(handler, c.middlewares...)

	for {
		if err := c.group.Consume(ctx, c.topics, newGroupHandler); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}

			slog.ErrorContext(ctx, "ошибка потребления Kafka", "error", err)
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// После ребалансировки sarama завершает текущий Consume и нужно
		// вызвать его повторно, чтобы получить новые назначенные партиции
		slog.InfoContext(ctx, "ребалансировка Kafka consumer group...")
	}
}
