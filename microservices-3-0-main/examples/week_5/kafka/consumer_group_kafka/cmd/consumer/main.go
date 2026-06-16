// Consumer Group — потребитель Kafka, работающий в составе группы
//
// Kafka Consumer Group — это механизм параллельного чтения из топика несколькими
// потребителями. Kafka сама распределяет партиции между потребителями группы:
//   - Если в группе 1 потребитель и 3 партиции — он читает все 3.
//   - Если в группе 3 потребителя и 3 партиции — каждый читает по одной
//   - Если потребителей больше, чем партиций — лишние простаивают
//
// При добавлении/удалении потребителя происходит ребалансировка — Kafka
// перераспределяет партиции между оставшимися членами группы
//
// Sarama требует реализовать интерфейс ConsumerGroupHandler с тремя методами:
//   - Setup    — вызывается после присоединения к группе (до начала чтения)
//   - ConsumeClaim — основной цикл: читаем сообщения из назначенной партиции
//   - Cleanup  — вызывается перед выходом из группы (при ребалансировке или остановке)
//
// Демо ребалансировки: в docker-compose топик создаётся с 3 партициями
// Запустите несколько экземпляров этого потребителя с разными KAFKA_CONSUMER_ID —
// Kafka автоматически распределит партиции между ними
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/IBM/sarama"
)

// Consumer реализует интерфейс sarama.ConsumerGroupHandler
// ready — канал-сигнал: закрывается в Setup(), когда потребитель готов к работе
// Это позволяет main() дождаться реальной готовности перед логированием
type Consumer struct {
	ready chan struct{}
	id    string
}

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "test-topic")
	groupID := cmp.Or(os.Getenv("KAFKA_GROUP_ID"), "consumer-group")
	consumerID := cmp.Or(os.Getenv("KAFKA_CONSUMER_ID"), "consumer-1")

	slog.Info("запуск Sarama-потребителя", "consumer_id", consumerID)

	// signal.NotifyContext = context.WithCancel + signal.Notify в одном вызове
	// При получении SIGINT/SIGTERM контекст автоматически отменяется,
	// что приводит к выходу из client.Consume() и завершению программы
	//
	// stop — это по сути тот же cancel из context.WithCancel, но дополнительно
	// отписывается от перехвата сигналов. Вызываем после <-ctx.Done(), чтобы
	// повторный Ctrl+C завершил процесс стандартным образом (без перехвата)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	config := sarama.NewConfig()
	// Версия протокола Kafka (не путать с версией Docker-образа!)
	// В docker-compose мы используем образ cp-kafka:8.2.0 — это версия платформы Confluent,
	// внутри которой работает Kafka 3.8.x. Версия протокола должна быть <= версии брокера
	config.Version = sarama.V3_6_0_0
	// Стратегия распределения партиций между потребителями группы
	// RoundRobin — равномерно раздаёт партиции по кругу
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	// С какого offset'а начинать чтение, если для группы ещё нет сохранённого offset'а
	// OffsetOldest — читать с самого начала (не пропускать старые сообщения)
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer := Consumer{
		ready: make(chan struct{}),
		id:    consumerID,
	}

	// NewConsumerGroup подключается к брокерам и регистрирует потребителя в группе
	// groupID — имя группы. Все потребители с одинаковым groupID делят партиции между собой
	client, err := sarama.NewConsumerGroup(strings.Split(brokerAddr, ","), groupID, config)
	if err != nil {
		slog.Error("не удалось создать клиент consumer group", "consumer_id", consumerID, "error", err)
		os.Exit(1)
	}

	// consume() блокируется в цикле, пока ctx не отменится
	// Запускаем в горутине, чтобы main() мог дождаться ready-сигнала
	go consume(ctx, client, consumer, topicName)

	// Ждём, пока Setup() закроет канал ready — это значит, что потребитель
	// присоединился к группе и получил назначенные партиции
	<-consumer.ready
	slog.Info("✅ потребитель запущен и работает, ожидание сообщений из топика", "consumer_id", consumerID, "topic", topicName)

	// Блокируемся до получения сигнала завершения (Ctrl+C или kill)
	<-ctx.Done()
	stop()
	slog.Info("завершение по сигналу", "consumer_id", consumerID)

	if err = client.Close(); err != nil {
		slog.Error("не удалось закрыть клиент consumer group", "consumer_id", consumerID, "error", err)
		os.Exit(1)
	}
}

// consume вызывает client.Consume() в цикле
// Зачем цикл? Потому что client.Consume() завершается после каждой ребалансировки
// После ребалансировки нужно вызвать его снова, чтобы продолжить чтение
// из новых назначенных партиций
func consume(ctx context.Context, client sarama.ConsumerGroup, consumer Consumer, topicName string) {
	for {
		// Consume блокируется до ребалансировки или отмены контекста
		// Внутри вызывает Setup → ConsumeClaim (для каждой партиции) → Cleanup
		err := client.Consume(ctx, strings.Split(topicName, ","), &consumer)
		if err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return
			}
			slog.Error("ошибка потребления", "consumer_id", consumer.id, "error", err)
			os.Exit(1)
		}

		// Если контекст отменён (по сигналу) — выходим из цикла
		if ctx.Err() != nil {
			return
		}

		// Если дошли сюда — произошла ребалансировка. Пересоздаём канал ready,
		// чтобы следующий вызов Setup() мог его снова закрыть
		slog.Info("ребалансировка", "consumer_id", consumer.id)
		consumer.ready = make(chan struct{})
	}
}

// Setup вызывается Kafka после присоединения к группе, ДО начала чтения сообщений
// Закрываем канал ready — это сигнал для main(), что потребитель готов
func (c *Consumer) Setup(sarama.ConsumerGroupSession) error {
	close(c.ready)
	slog.Info("потребитель готов", "consumer_id", c.id)
	return nil
}

// Cleanup вызывается перед выходом потребителя из группы (при ребалансировке или остановке)
// Здесь можно освободить ресурсы, закрыть соединения и т.д
func (c *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	slog.Info("очистка потребителя", "consumer_id", c.id)
	return nil
}

// ConsumeClaim — основной цикл обработки сообщений из одной партиции
// Kafka вызывает этот метод для каждой партиции, назначенной потребителю
// Если потребителю назначены 3 партиции — будет 3 параллельных вызова ConsumeClaim
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				// Канал закрыт — партиция больше не назначена этому потребителю
				slog.Info("канал сообщений закрыт", "consumer_id", c.id)
				return nil
			}

			slog.Info("получено сообщение", "consumer_id", c.id, "value", string(message.Value), "partition", message.Partition, "offset", message.Offset)

			// MarkMessage сообщает Kafka, что сообщение обработано
			// Kafka запоминает offset для группы — при перезапуске чтение продолжится
			// с последнего подтверждённого offset'а, а не с начала
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			// Сессия завершена (ребалансировка или остановка)
			return nil
		}
	}
}
