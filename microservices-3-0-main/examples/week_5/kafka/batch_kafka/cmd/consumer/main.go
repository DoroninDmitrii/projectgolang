// Поштучный потребитель — обрабатывает каждое сообщение по отдельности
//
// Это стандартный подход, где ConsumeClaim читает сообщения из канала
// и обрабатывает их одно за другим: прочитал → обработал → подтвердил → следующее
//
// Подходит для случаев, когда:
//   - Каждое сообщение нужно обработать независимо
//   - Объём сообщений небольшой
//   - Нет необходимости в оптимизации (например, батчевой вставке в БД)
//
// Для сравнения с батчевым подходом см. batch_consumer
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/IBM/sarama"
)

// Consumer реализует sarama.ConsumerGroupHandler
// Обрабатывает каждое сообщение по отдельности
type Consumer struct {
	ready     chan struct{}
	processed atomic.Int64
	startTime time.Time
}

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "batch-topic")
	groupID := cmp.Or(os.Getenv("KAFKA_GROUP_ID"), "one-by-one-group")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	config := sarama.NewConfig()
	config.Version = sarama.V3_6_0_0
	// Стратегия ребалансировки определяет, как партиции распределяются
	// между консюмерами внутри одной consumer group
	//
	// RoundRobin — распределяет партиции по кругу глобально по всем топикам:
	//   2 топика × 3 партиции, 2 консюмера:
	//     Consumer 1: topicA-0, topicA-2, topicB-1  (3 партиции)
	//     Consumer 2: topicA-1, topicB-0, topicB-2  (3 партиции)
	//
	// Range (дефолт) — делит партиции блоками per-topic, что может давать перекос:
	//   2 топика × 3 партиции, 2 консюмера:
	//     Consumer 1: topicA-0, topicA-1, topicB-0, topicB-1  (4 партиции)
	//     Consumer 2: topicA-2, topicB-2                        (2 партиции)
	//
	// Ребалансировка происходит когда:
	//   - Консюмер присоединился или вышел из группы (упал, таймаут heartbeat)
	//   - Добавились новые партиции в топик
	// На время ребалансировки все консюмеры группы приостанавливают чтение
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer := &Consumer{
		ready: make(chan struct{}),
	}

	client, err := sarama.NewConsumerGroup(strings.Split(brokerAddr, ","), groupID, config)
	if err != nil {
		slog.Error("не удалось создать consumer group", "error", err)
		os.Exit(1)
	}

	go consume(ctx, client, consumer, topicName)

	<-consumer.ready
	slog.Info("✅ потребитель запущен и работает, ожидание сообщений из топика",
		"topic", topicName, "mode", "поштучная обработка")

	<-ctx.Done()
	stop()

	slog.Info("итого обработано сообщений",
		"count", consumer.processed.Load(),
		"elapsed", time.Since(consumer.startTime))

	if err = client.Close(); err != nil {
		slog.Error("не удалось закрыть consumer group", "error", err)
	}
}

func consume(ctx context.Context, client sarama.ConsumerGroup, consumer *Consumer, topicName string) {
	for {
		err := client.Consume(ctx, strings.Split(topicName, ","), consumer)
		if err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return
			}
			slog.Error("ошибка потребления", "error", err)
			os.Exit(1)
		}

		if ctx.Err() != nil {
			return
		}

		consumer.ready = make(chan struct{})
	}
}

// Setup вызывается при присоединении к группе
func (c *Consumer) Setup(sarama.ConsumerGroupSession) error {
	c.startTime = time.Now()
	close(c.ready)
	return nil
}

// Cleanup вызывается при выходе из группы
func (c *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim обрабатывает каждое сообщение по одному
// Каждое сообщение: прочитали → залогировали → подтвердили offset
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			slog.Info("обработано сообщение",
				"value", string(msg.Value),
				"partition", msg.Partition,
				"offset", msg.Offset)

			session.MarkMessage(msg, "")
			c.processed.Add(1)

		case <-session.Context().Done():
			return nil
		}
	}
}
