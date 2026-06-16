package producer

import (
	"context"
	"log/slog"

	"github.com/IBM/sarama"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka"
)

// Producer — обёртка над sarama.SyncProducer для отправки сообщений в конкретный топик
//
// ВАЖНО: sarama.SyncProducer требует sarama.Config с Producer.Return.Successes = true,
// иначе SendMessage зависнет навсегда. Убедитесь, что эта настройка выставлена
// при создании sarama.Config (обычно в конфигурационном слое сервиса)
type Producer struct {
	syncProducer sarama.SyncProducer
	topic        string
}

// NewProducer создаёт Producer, привязанный к конкретному топику
// syncProducer должен быть создан с Producer.Return.Successes = true
func NewProducer(syncProducer sarama.SyncProducer, topic string) *Producer {
	return &Producer{
		syncProducer: syncProducer,
		topic:        topic,
	}
}

// Send синхронно отправляет сообщение в Kafka и блокируется до получения ACK от брокера
// Возвращает ошибку, если брокер не подтвердил запись
func (p *Producer) Send(ctx context.Context, msg *kafka.Message) error {
	saramaMsg := &sarama.ProducerMessage{
		Topic:   p.topic,
		Key:     sarama.ByteEncoder(msg.Key),
		Value:   sarama.ByteEncoder(msg.Value),
		Headers: convertHeaders(msg.Headers),
	}

	partition, offset, err := p.syncProducer.SendMessage(saramaMsg)
	if err != nil {
		slog.ErrorContext(ctx, "не удалось отправить сообщение", "error", err)
		return err
	}

	slog.InfoContext(
		ctx, "сообщение отправлено",
		"topic", p.topic,
		"partition", partition,
		"offset", offset,
		"key", string(msg.Key),
		"value_size", len(msg.Value),
	)

	return nil
}

func convertHeaders(headers []kafka.Header) []sarama.RecordHeader {
	if len(headers) == 0 {
		return nil
	}

	result := make([]sarama.RecordHeader, 0, len(headers))
	for _, h := range headers {
		result = append(result, sarama.RecordHeader{
			Key:   []byte(h.Key),
			Value: h.Value,
		})
	}

	return result
}
