// Sync Producer — синхронный продюсер Kafka
//
// SendMessage блокирует вызывающий код до тех пор, пока брокер не подтвердит запись
// Это значит, что при отправке N сообщений суммарное время ≈ N * latency,
// потому что каждое следующее сообщение ждёт подтверждения предыдущего
//
// Главное преимущество перед async: ты сразу знаешь результат
// Если сообщение #3 не записалось — ты можешь остановиться и не слать #4.
// В async такой контроль невозможен: пока ты узнал об ошибке, следующие уже в пути
//
// Когда это актуально:
//   - Нужна гарантия, что сообщение записано ДО того, как продолжится бизнес-логика
//     (например, "создали заказ → отправили событие → ответили клиенту")
//   - Нужна семантика "упало → стоп" (не отправлять следующее при ошибке)
//   - Объём сообщений невелик или задержка не критична
//   - Простота кода важнее пропускной способности
package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/IBM/sarama"
	"github.com/brianvoe/gofakeit/v7"
)

const messageCount = 10

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "test-topic")

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

	slog.Info("запуск sync-продюсера", "broker", brokerAddr, "topic", topicName, "count", messageCount)

	// Каждый SendMessage блокируется до получения ack от брокера
	// Следующее сообщение не отправляется, пока предыдущее не подтверждено
	start := time.Now()
	for i := range messageCount {
		message := fmt.Sprintf("sync-%d: %s", i+1, gofakeit.StreetName())
		msg := &sarama.ProducerMessage{
			Topic: topicName,
			Value: sarama.StringEncoder(message),
		}

		var partition int32
		var offset int64
		partition, offset, err = producer.SendMessage(msg)
		if err != nil {
			slog.Error("не удалось отправить сообщение в Kafka", "error", err)
			return
		}

		slog.Info("сообщение успешно отправлено",
			"message_num", i+1, "partition", partition, "offset", offset, "elapsed", time.Since(start))
	}

	slog.Info("все сообщения отправлены", "count", messageCount, "elapsed", time.Since(start))
}

// newSyncProducer создаёт синхронный Kafka-продюсер с подтверждением от всех реплик
func newSyncProducer(brokerList []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer(brokerList, config)
	if err != nil {
		return nil, err
	}

	return producer, nil
}
