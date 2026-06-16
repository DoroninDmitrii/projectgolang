// Sync Producer для демонстрации Consumer Group
//
// Отправляет пачку сообщений в Kafka-топик, чтобы наглядно увидеть распределение
// по партициям. Фокус этого примера — на потребителе (consumer group с ребалансировкой),
// поэтому продюсер максимально простой
// Подробный разбор sync producer — см. пример easy_kafka
package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"

	"github.com/IBM/sarama"
	"github.com/brianvoe/gofakeit/v7"
)

// messageCount — количество сообщений, отправляемых за один запуск продюсера
// 20 сообщений при 3 партициях дают примерно по 6-7 сообщений на партицию,
// что позволяет наглядно увидеть распределение по партициям в consumer group
const messageCount = 20

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

	slog.Info("отправка сообщений в топик", "topic", topicName, "count", messageCount)

	for i := range messageCount {
		message := fmt.Sprintf("#%d %s", i+1, gofakeit.StreetName())
		msg := &sarama.ProducerMessage{
			Topic: topicName,
			Value: sarama.StringEncoder(message),
		}

		partition, offset, err := producer.SendMessage(msg)
		if err != nil {
			slog.Error("не удалось отправить сообщение", "index", i+1, "error", err)
			continue
		}

		slog.Info("сообщение отправлено", "index", i+1, "message", message, "partition", partition, "offset", offset)
	}

	slog.Info("все сообщения отправлены", "count", messageCount)
}

func newSyncProducer(brokerList []string) (sarama.SyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true

	// Стратегия выбора партиции для сообщений
	//
	// По умолчанию Sarama использует HashPartitioner — он вычисляет hash от ключа
	// сообщения и по нему определяет партицию. Проблема: если ключ не задан (nil),
	// hash(nil) всегда даёт одно и то же число, и ВСЕ сообщения без ключа летят
	// в одну и ту же партицию. Остальные партиции простаивают
	//
	// RoundRobinPartitioner распределяет сообщения по партициям по кругу:
	// 1-е сообщение → партиция 0, 2-е → партиция 1, 3-е → партиция 2, 4-е → партиция 0, ...
	// Это даёт равномерную нагрузку и позволяет наглядно увидеть, как consumer group
	// читает из разных партиций параллельно
	//
	// Альтернатива: если важен порядок обработки сообщений одной сущности —
	// используйте HashPartitioner (по умолчанию) + явный ключ:
	//
	//   msg := &sarama.ProducerMessage{
	//       Topic: "my-topic",
	//       Key:   sarama.StringEncoder(userID),  // все сообщения одного пользователя → в одну партицию
	//       Value: sarama.StringEncoder(payload),
	//   }
	//
	// Тогда сообщения с одинаковым ключом гарантированно попадают в одну партицию,
	// а значит — обрабатываются одним потребителем и строго по порядку
	config.Producer.Partitioner = sarama.NewRoundRobinPartitioner

	producer, err := sarama.NewSyncProducer(brokerList, config)
	if err != nil {
		return nil, err
	}

	return producer, nil
}
