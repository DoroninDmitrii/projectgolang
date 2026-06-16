// Simple Consumer — простейший потребитель Kafka
//
// Читает сообщения напрямую из конкретной партиции топика, без consumer group
// В отличие от consumer group, здесь:
//   - Нет автоматического распределения партиций между потребителями
//   - Нет ребалансировки — каждый потребитель сам указывает, какую партицию читать
//   - Нет автоматического коммита offset'ов — Kafka не запоминает, где мы остановились
//     При перезапуске чтение начнётся с OffsetOldest или OffsetNewest, а не с последнего места
//
// Это самый базовый способ чтения из Kafka. Подходит для:
//   - Знакомства с Kafka (понять, как устроены партиции и offset'ы)
//   - Простых утилит и скриптов (прочитать все сообщения из партиции)
//   - Отладки (посмотреть, что лежит в конкретной партиции)
//
// Для продакшена используется Consumer Group — см. пример consumer_group_kafka
package main

import (
	"cmp"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/IBM/sarama"
)

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "test-topic")

	// signal.NotifyContext = context.WithCancel + signal.Notify в одном вызове
	// При получении SIGINT/SIGTERM контекст автоматически отменяется
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	// sarama.NewConsumer создаёт простой потребитель (без группы)
	// Он не управляет offset'ами и не участвует в ребалансировке
	consumer, err := sarama.NewConsumer([]string{brokerAddr}, nil)
	if err != nil {
		slog.Error("не удалось создать потребитель", "error", err)
		os.Exit(1)
	}

	// ConsumePartition подписывается на конкретную партицию (0) с конкретного offset'а
	// OffsetOldest — читать с самого начала партиции (все накопленные сообщения)
	// OffsetNewest — читать только новые сообщения (пропустить старые)
	partitionConsumer, err := consumer.ConsumePartition(topicName, 0, sarama.OffsetOldest)
	if err != nil {
		slog.Error("не удалось подписаться на партицию", "topic", topicName, "error", err)

		if closeErr := consumer.Close(); closeErr != nil {
			slog.Error("не удалось закрыть consumer", "error", closeErr)
		}

		os.Exit(1)
	}

	slog.Info("✅ потребитель запущен и работает, ожидание сообщений из топика",
		"topic", topicName, "partition", 0)

	// Читаем сообщения до получения сигнала завершения (Ctrl+C или kill)
	for {
		select {
		case msg := <-partitionConsumer.Messages():
			slog.Info("получено сообщение",
				"value", string(msg.Value),
				"partition", msg.Partition,
				"offset", msg.Offset)
		case err = <-partitionConsumer.Errors():
			slog.Error("ошибка чтения", "error", err)
		case <-ctx.Done():
			stop()

			// Закрываем в правильном порядке: сначала partitionConsumer, потом consumer
			if closeErr := partitionConsumer.Close(); closeErr != nil {
				slog.Error("не удалось закрыть partition consumer", "error", closeErr)
			}

			if closeErr := consumer.Close(); closeErr != nil {
				slog.Error("не удалось закрыть consumer", "error", closeErr)
			}

			return
		}
	}
}
