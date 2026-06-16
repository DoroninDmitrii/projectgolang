// Сравнение sync и async продюсеров Kafka
//
// Запускает оба варианта подряд с одинаковым количеством сообщений
// и выводит результат — наглядно видна разница во времени
package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/brianvoe/gofakeit/v7"
)

const messageCount = 20

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "test-topic")
	brokers := []string{brokerAddr}

	slog.Info("=== сравнение sync vs async продюсеров ===",
		"broker", brokerAddr, "topic", topicName, "count", messageCount)

	syncDuration := benchSync(brokers, topicName)
	asyncDuration := benchAsync(brokers, topicName)

	slog.Info(
		"=== результат ===",
		"sync", syncDuration,
		"async", asyncDuration,
		"speedup", fmt.Sprintf("%.1fx", float64(syncDuration)/float64(asyncDuration)),
	)
}

// benchSync отправляет messageCount сообщений синхронно (по одному, с ожиданием ack)
func benchSync(brokers []string, topic string) time.Duration {
	producer, err := newSyncProducer(brokers)
	if err != nil {
		slog.Error("sync: не удалось создать продюсер", "error", err)
		os.Exit(1)
	}

	slog.Info("[sync] старт: каждое сообщение ждёт ack от брокера", "count", messageCount)

	start := time.Now()
	for i := range messageCount {
		msg := &sarama.ProducerMessage{
			Topic: topic,
			Value: sarama.StringEncoder(fmt.Sprintf("sync-%d: %s", i+1, gofakeit.StreetName())),
		}

		_, _, err = producer.SendMessage(msg)
		if err != nil {
			slog.Error("sync: ошибка отправки", "error", err)
			os.Exit(1)
		}
	}
	elapsed := time.Since(start)
	if err = producer.Close(); err != nil {
		slog.Error("sync: не удалось закрыть продюсер", "error", err)
	}

	slog.Info("[sync] готово", "elapsed", elapsed)
	return elapsed
}

// benchAsync отправляет messageCount сообщений асинхронно (все в канал, ack параллельно)
func benchAsync(brokers []string, topic string) time.Duration {
	producer, err := newAsyncProducer(brokers)
	if err != nil {
		slog.Error("async: не удалось создать продюсер", "error", err)
		os.Exit(1)
	}

	slog.Info("[async] старт: все сообщения в канал, ack приходят параллельно", "count", messageCount)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range messageCount {
			select {
			case <-producer.Successes():
			case fail := <-producer.Errors():
				slog.Error("async: ошибка отправки", "error", fail.Err)
			}
		}
	}()

	start := time.Now()
	for i := range messageCount {
		msg := &sarama.ProducerMessage{
			Topic: topic,
			Value: sarama.StringEncoder(fmt.Sprintf("async-%d: %s", i+1, gofakeit.StreetName())),
		}
		producer.Input() <- msg
	}

	wg.Wait()
	elapsed := time.Since(start)
	producer.AsyncClose()

	slog.Info("[async] готово", "elapsed", elapsed)
	return elapsed
}

func newSyncProducer(brokerList []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true

	return sarama.NewSyncProducer(brokerList, config)
}

func newAsyncProducer(brokerList []string) (sarama.AsyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	return sarama.NewAsyncProducer(brokerList, config)
}
