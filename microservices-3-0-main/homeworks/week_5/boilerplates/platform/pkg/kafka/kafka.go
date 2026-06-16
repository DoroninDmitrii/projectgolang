package kafka

import (
	"context"
)

// MessageHandler — функция-обработчик входящего Kafka-сообщения
//
// Используется как конечный обработчик в consumer-pipeline:
//
//	handler := func(ctx context.Context, msg kafka.Message) error {
//	    // декодировать msg.Value, обработать, вернуть ошибку при неудаче
//	}
//
// При возврате ошибки сообщение НЕ коммитится (at-least-once семантика)
type MessageHandler func(ctx context.Context, msg Message) error

// Middleware — обёртка для MessageHandler, позволяющая добавлять сквозную логику
// (логирование, метрики, трейсинг) без изменения бизнес-обработчика
//
// Middleware применяются в порядке «снаружи внутрь»: первый добавленный middleware
// оборачивает всю цепочку, последний — ближайший к бизнес-обработчику
type Middleware func(next MessageHandler) MessageHandler
