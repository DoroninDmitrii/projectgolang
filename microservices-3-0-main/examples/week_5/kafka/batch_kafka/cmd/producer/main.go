// Массовый продюсер для демонстрации батчевой обработки
//
// Отправляет большое количество сообщений в топик с 3 партициями,
// чтобы наглядно сравнить поштучную и батчевую обработку на стороне потребителя
//
// RoundRobinPartitioner распределяет сообщения равномерно по партициям,
// чтобы каждый потребитель получил примерно одинаковое количество сообщений
package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/IBM/sarama"
	"github.com/brianvoe/gofakeit/v7"
)

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "batch-topic")

	messageCount, err := strconv.Atoi(cmp.Or(os.Getenv("MESSAGE_COUNT"), "500"))
	if err != nil {
		slog.Error("некорректное значение MESSAGE_COUNT", "error", err)
		os.Exit(1)
	}

	producer, err := newSyncProducer([]string{brokerAddr})
	if err != nil {
		slog.Error("не удалось запустить продюсер", "error", err)
		os.Exit(1)
	}

	defer func() {
		if err = producer.Close(); err != nil {
			slog.Error("не удалось закрыть продюсер", "error", err)
		}
	}()

	slog.Info("отправка сообщений", "broker", brokerAddr, "topic", topicName, "count", messageCount)

	start := time.Now()
	for i := range messageCount {
		msg := &sarama.ProducerMessage{
			Topic: topicName,
			Value: sarama.StringEncoder(fmt.Sprintf("#%d город=%s", i+1, gofakeit.City())),
		}

		_, _, err = producer.SendMessage(msg)
		if err != nil {
			slog.Error("не удалось отправить сообщение", "index", i+1, "error", err)
			continue
		}
	}

	slog.Info("все сообщения отправлены", "count", messageCount, "elapsed", time.Since(start))
}

func newSyncProducer(brokerList []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true
	// RoundRobin для равномерного распределения по партициям (без ключа)
	config.Producer.Partitioner = sarama.NewRoundRobinPartitioner

	return sarama.NewSyncProducer(brokerList, config)
}
