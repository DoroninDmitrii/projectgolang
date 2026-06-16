package kafka

import (
	"context"

	"github.com/student/platform/pkg/auth"
	"github.com/student/platform/pkg/kafka"
)

// SessionHeaderKey — имя Kafka-заголовка, через который сервисы прокидывают
// UUID пользовательской сессии между собой. Совпадает с gRPC metadata-ключом
// (см. order/internal/interceptor/auth.go), чтобы один и тот же session-uuid
// шёл по всей цепочке HTTP → Kafka → gRPC под одним именем.
const SessionHeaderKey = "session-uuid"

// ProducerSessionHeaders собирает Kafka-заголовки для исходящего сообщения, доставая
// session-uuid из context. Если в context'е session-uuid нет — возвращает nil
// (заголовок не будет добавлен, и consumer на другой стороне не сможет
// восстановить session-uuid, а защищённые gRPC-вызовы из обработчика упадут
// с Unauthenticated).
//
// Использование в producer-сервисе:
//
//	return s.producer.Send(ctx, &kafka.Message{
//	    Key:     key,
//	    Value:   payload,
//	    Headers: kafkamw.ProducerSessionHeaders(ctx),
//	})
func ProducerSessionHeaders(ctx context.Context) []kafka.Header {
	sessionUUID, ok := auth.SessionUUIDFromContext(ctx)
	if !ok || sessionUUID == "" {
		return nil
	}
	return []kafka.Header{
		{Key: SessionHeaderKey, Value: []byte(sessionUUID)},
	}
}

// ConsumerSession — Kafka middleware, которое читает session-uuid из заголовков
// сообщения и кладёт его в context перед вызовом основного обработчика.
//
// Зачем: SessionForwarder gRPC-клиента (см. order/internal/interceptor/auth.go)
// автоматически берёт session-uuid из context и пробрасывает в исходящую
// metadata. Без этого middleware Kafka-handler получает «голый» context от
// sarama.ConsumerGroupSession без session-uuid, и любой защищённый gRPC-вызов
// (например, InventoryService.CommitParts) вернёт codes.Unauthenticated.
//
// Если заголовка нет — context не модифицируется. Это сознательное решение:
// middleware не должно решать за бизнес-обработчик, что делать с отсутствием
// сессии (отбросить, отдать на DLQ, выполнить под service-identity) — это
// зона ответственности конкретного сервиса.
func ConsumerSession() kafka.Middleware {
	return func(next kafka.MessageHandler) kafka.MessageHandler {
		return func(ctx context.Context, msg kafka.Message) error {
			for _, h := range msg.Headers {
				if h.Key == SessionHeaderKey && len(h.Value) > 0 {
					ctx = auth.WithSessionUUID(ctx, string(h.Value))
					break
				}
			}
			return next(ctx, msg)
		}
	}
}
