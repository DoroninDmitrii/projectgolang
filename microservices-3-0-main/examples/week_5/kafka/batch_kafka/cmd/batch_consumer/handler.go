package main

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
)

// BatchConsumer реализует sarama.ConsumerGroupHandler с батчевой обработкой
type BatchConsumer struct {
	ready         chan struct{}
	batchSize     int
	flushInterval time.Duration

	// Статистика — атомарные счётчики, потому что ConsumeClaim
	// вызывается параллельно для разных партиций
	processed  atomic.Int64
	batchCount atomic.Int64
	startTime  time.Time
}

// Setup вызывается при присоединении к группе
func (c *BatchConsumer) Setup(sarama.ConsumerGroupSession) error {
	c.startTime = time.Now()
	close(c.ready)
	return nil
}

// Cleanup вызывается при выходе из группы
func (c *BatchConsumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim читает сообщения из партиции и накапливает их в батч
//
// Для каждой партиции создаётся свой буфер — это правильно, потому что:
//   - offset'ы в Kafka привязаны к партиции, и подтверждать нужно per-partition
//   - Sarama вызывает ConsumeClaim параллельно для каждой партиции
//   - Shared-буфер потребовал бы синхронизации и усложнил бы подтверждение offset'ов
func (c *BatchConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	batch := make([]*sarama.ConsumerMessage, 0, c.batchSize)
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	// flush обрабатывает накопленный батч и подтверждает offset'ы
	// Если обработка завершилась с ошибкой — сообщения НЕ маркаются,
	// и будут переобработаны после rebalance
	flush := func() {
		if len(batch) == 0 {
			return
		}

		if err := processBatch(batch); err != nil {
			slog.Error("ошибка обработки батча, сообщения не подтверждены — будут переобработаны",
				"error", err,
				"batch_size", len(batch),
				"partition", batch[0].Partition)
			return
		}

		// Подтверждаем offset для каждого сообщения в батче
		// MarkMessage безопасно вызывать для любого сообщения — Kafka запомнит
		// максимальный offset, поэтому порядок вызовов не важен
		for _, msg := range batch {
			session.MarkMessage(msg, "")
		}

		c.processed.Add(int64(len(batch)))
		c.batchCount.Add(1)

		// Очищаем буфер, сохраняя выделенную память (capacity)
		batch = batch[:0]
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				flush()
				return nil
			}

			batch = append(batch, msg)

			if len(batch) >= c.batchSize {
				flush()
				// Сбрасываем таймер: только что сделали flush по размеру,
				// следующий flush по таймеру нужен через полный интервал
				ticker.Reset(c.flushInterval)
			}

		case <-ticker.C:
			// Таймер сработал — flush неполного буфера
			// Это гарантирует, что сообщения не "зависнут" в буфере надолго
			flush()

		case <-session.Context().Done():
			// Сессия завершена — обрабатываем оставшиеся сообщения
			flush()
			return nil
		}
	}
}
