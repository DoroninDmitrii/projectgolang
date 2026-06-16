// Batch Consumer — потребитель Kafka с батчевой обработкой сообщений
//
// Вместо обработки каждого сообщения по отдельности, этот потребитель
// накапливает сообщения в буфер и обрабатывает их пачкой
//
// Flush (сброс буфера) происходит по двум условиям:
//  1. Буфер достиг batchSize — набралось достаточно сообщений
//  2. Прошёл flushInterval — таймер сработал для неполного буфера
//
// Зачем это нужно:
//   - Батчевая вставка в БД (INSERT ... VALUES (...), (...), (...)) в разы быстрее
//     поштучной (INSERT ... VALUES (...)), потому что один round-trip к БД вместо N
//   - Батчевая отправка в другой сервис (gRPC, HTTP) — меньше сетевых вызовов
//   - Любая операция с фиксированным overhead выигрывает от батчинга:
//     N операций с overhead 1мс = N*1мс, 1 батч-операция с overhead 1мс = 1мс
//
// Каждая партиция получает свой буфер — ConsumeClaim вызывается отдельно для
// каждой назначенной партиции. Это правильно: offset'ы в Kafka per-partition,
// поэтому и подтверждение должно быть per-partition
//
// flushInterval нужен, чтобы неполный буфер не "застрял" навечно
// Например: batchSize=50, пришло 30 сообщений, новых нет. Без таймера эти 30
// будут ждать ещё 20 сообщений бесконечно. С таймером — обработаются через 2с
//
// Для сравнения с поштучной обработкой см. consumer
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/IBM/sarama"
)

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "batch-topic")
	groupID := cmp.Or(os.Getenv("KAFKA_GROUP_ID"), "batch-group")
	batchSize, err := strconv.Atoi(cmp.Or(os.Getenv("BATCH_SIZE"), "50"))
	if err != nil {
		slog.Error("некорректное значение BATCH_SIZE", "error", err)
		os.Exit(1)
	}

	flushIntervalSec, err := strconv.Atoi(cmp.Or(os.Getenv("FLUSH_INTERVAL_SEC"), "2"))
	if err != nil {
		slog.Error("некорректное значение FLUSH_INTERVAL_SEC", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	config := sarama.NewConfig()
	config.Version = sarama.V3_6_0_0
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer := &BatchConsumer{
		ready:         make(chan struct{}),
		batchSize:     batchSize,
		flushInterval: time.Duration(flushIntervalSec) * time.Second,
	}

	client, err := sarama.NewConsumerGroup(strings.Split(brokerAddr, ","), groupID, config)
	if err != nil {
		slog.Error("не удалось создать consumer group", "error", err)
		os.Exit(1)
	}

	go consume(ctx, client, consumer, topicName)

	<-consumer.ready
	slog.Info("✅ потребитель запущен и работает, ожидание сообщений из топика",
		"topic", topicName, "mode", "батчевая обработка",
		"batch_size", batchSize, "flush_interval", consumer.flushInterval)

	<-ctx.Done()
	stop()

	slog.Info("итого обработано",
		"сообщений", consumer.processed.Load(),
		"батчей", consumer.batchCount.Load(),
		"elapsed", time.Since(consumer.startTime))

	if err = client.Close(); err != nil {
		slog.Error("не удалось закрыть consumer group", "error", err)
	}
}

func consume(ctx context.Context, client sarama.ConsumerGroup, consumer *BatchConsumer, topicName string) {
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
