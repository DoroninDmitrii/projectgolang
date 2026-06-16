package consumer

import (
	"log/slog"

	"github.com/IBM/sarama"

	"github.com/student/platform/pkg/kafka"
)

// groupHandler реализует sarama.ConsumerGroupHandler
//
// Жизненный цикл (вызывается sarama при каждой ребалансировке):
//  1. Setup    — вызывается один раз при получении партиций
//  2. ConsumeClaim — вызывается для каждой назначенной партиции (в отдельной горутине)
//  3. Cleanup  — вызывается после завершения всех ConsumeClaim
type groupHandler struct {
	handler kafka.MessageHandler
}

// NewGroupHandler создаёт groupHandler, оборачивая handler в middleware-цепочку
//
// Middleware применяются в обратном порядке (последний — ближайший к handler),
// чтобы порядок вызова совпадал с порядком передачи:
// WithMiddlewares(logging, metrics) → logging → metrics → handler
func NewGroupHandler(handler kafka.MessageHandler, middlewares ...kafka.Middleware) *groupHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return &groupHandler{
		handler: handler,
	}
}

func (g *groupHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (g *groupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim читает сообщения из одной партиции до её закрытия или отмены сессии
//
// At-least-once семантика: MarkMessage вызывается только после успешной обработки
// При ошибке обработчика сообщение логируется и пропускается — при перезапуске
// consumer group оно будет доставлено повторно (если оффсет не был закоммичен)
func (g *groupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				slog.InfoContext(session.Context(), "канал сообщений Kafka закрыт")
				return nil
			}

			msg := kafka.Message{
				Key:       message.Key,
				Value:     message.Value,
				Topic:     message.Topic,
				Partition: message.Partition,
				Offset:    message.Offset,
				Timestamp: message.Timestamp,
				Headers:   extractHeaders(message.Headers),
			}

			if err := g.handler(session.Context(), msg); err != nil {
				slog.ErrorContext(session.Context(), "ошибка обработчика Kafka", "error", err)
				continue
			}

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			slog.InfoContext(session.Context(), "контекст сессии Kafka завершён")
			return nil
		}
	}
}

func extractHeaders(headers []*sarama.RecordHeader) []kafka.Header {
	result := make([]kafka.Header, 0, len(headers))
	for _, h := range headers {
		if h != nil && h.Key != nil {
			result = append(result, kafka.Header{
				Key:   string(h.Key),
				Value: h.Value,
			})
		}
	}

	return result
}
