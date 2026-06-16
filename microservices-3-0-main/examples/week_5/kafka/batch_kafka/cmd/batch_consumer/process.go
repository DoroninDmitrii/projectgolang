package main

import (
	"log/slog"

	"github.com/IBM/sarama"
)

// processBatch имитирует батчевую обработку
//
// В реальном коде здесь была бы:
//   - Батчевая вставка в PostgreSQL: INSERT INTO ... VALUES ($1), ($2), ..., ($N)
//   - Или батчевый вызов внешнего сервиса
//
// Важно: операция должна быть атомарной (в одной транзакции)
// Тогда "всё или ничего" — это фича: при ошибке ни одно сообщение
// не будет подтверждено, и весь батч будет переобработан
//
// Для демонстрации просто логируем сводку по батчу
func processBatch(batch []*sarama.ConsumerMessage) error { //nolint:unparam // В реальном коде здесь будет batch insert в БД, который может вернуть ошибку
	// Собираем информацию о партициях в батче для наглядности
	partitions := make(map[int32]int)
	for _, msg := range batch {
		partitions[msg.Partition]++
	}

	slog.Info("обработан батч",
		"размер", len(batch),
		"партиции", partitions,
		"первое", string(batch[0].Value),
		"последнее", string(batch[len(batch)-1].Value))

	// В реальном коде:
	// return db.BatchInsert(ctx, convertMessages(batch))
	return nil
}
