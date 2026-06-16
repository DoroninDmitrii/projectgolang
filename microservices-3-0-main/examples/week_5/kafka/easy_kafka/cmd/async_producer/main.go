// Async Producer — асинхронный продюсер Kafka
//
// В отличие от sync-продюсера, здесь отправка и подтверждение разделены:
//   - Сообщения кладутся в канал Input() — это НЕ блокирует до записи на брокер
//   - Подтверждения приходят асинхронно через каналы Successes() и Errors()
//
// Выигрыш — в пайплайнинге (pipelining):
//   - Sync:  отправил → ждёшь ack → отправил следующее → ждёшь ack → ...
//     Суммарное время ≈ N * latency
//   - Async: отправил все в канал (почти мгновенно) → подтверждения приходят параллельно
//     Суммарное время ≈ 1 * latency (при достаточном буфере и пропускной способности)
//
// Важно: async продюсер тоже получает подтверждения — он НЕ "отправил и забыл".
// Разница с sync — не в наличии подтверждений, а в том, КОГДА ты узнаёшь об ошибке:
//   - Sync:  отправил → сразу узнал результат → можешь остановиться и не слать следующее
//   - Async: отправил 10 сообщений → потом читаешь результаты → если #3 упало,
//     сообщения #4–#10 уже в пути. Остановить конвейер "на лету" нельзя
//
// Это не значит, что ошибки теряются — их можно ретраить через канал Errors()
// Sarama сама делает Retry.Max попыток (у нас 5), и только после этого шлёт в Errors()
// Но если нужна строгая семантика "упало → стоп", то лучше sync
//
// Есть и третий режим — настоящий "отправил и забыл":
// поставить Return.Successes=false и Return.Errors=false
// Тогда каналы вообще не нужно читать, но и об ошибках не узнаешь
//
// Что под капотом:
//   - Канал Input() — без буфера (unbuffered). Если внутренний пайплайн не успевает
//     разгребать, запись в Input() просто заблокируется — горутина-отправитель встанет
//     на строке `producer.Input() <- msg` и будет ждать, пока sarama не заберёт сообщение
//     Это и есть backpressure: быстрый отправитель автоматически притормаживается
//     до скорости обработки, без потери данных и без переполнения памяти
//   - Внутри sarama — цепочка (pipeline) горутин:
//     Input() → [1 dispatcher] → [1 topicProducer] → [1 partitionProducer] → [1 brokerProducer]
//     Это НЕ fan-out (много читателей из одного канала), а последовательная цепочка,
//     где у каждого канала ровно один читатель. На каждую партицию создаётся свой
//     partitionProducer со своим каналом и горутиной — поэтому partition-0 и partition-1
//     обрабатываются параллельно и независимо друг от друга
//     Каналы между уровнями буферизированы на ChannelBufferSize (по умолчанию 256
//     сообщений), что сглаживает пики нагрузки
//   - Порядок сообщений сохраняется в рамках одной партиции. Поскольку на каждом
//     этапе цепочки ровно одна горутина читает из FIFO-канала и передаёт дальше,
//     порядок не нарушается. Даже при ретраях partitionProducer буферизирует новые
//     сообщения, пока ретраи не завершатся, и только потом отправляет их — в исходном
//     порядке. Между разными партициями порядок не гарантирован
//
// Когда это актуально:
//   - Большой объём сообщений: нужно отправить много за короткое время (логи, метрики, события)
//   - Задержка критична: нельзя блокировать горячий путь на каждом сообщении
//   - Допустима чуть более сложная обработка ошибок (через отдельную горутину)
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

const messageCount = 10

func main() {
	brokerAddr := cmp.Or(os.Getenv("KAFKA_BROKER_ADDR"), "localhost:9092")
	topicName := cmp.Or(os.Getenv("KAFKA_TOPIC"), "test-topic")

	producer, err := newAsyncProducer([]string{brokerAddr})
	if err != nil {
		slog.Error("не удалось запустить async-продюсер", "error", err)
		os.Exit(1)
	}

	slog.Info("запуск async-продюсера", "broker", brokerAddr, "topic", topicName, "count", messageCount)

	// Отдельная горутина читает подтверждения и ошибки
	// Это обязательно при Return.Successes=true и Return.Errors=true —
	// если не читать каналы, продюсер заблокируется (deadlock)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range messageCount {
			select {
			case success := <-producer.Successes():
				slog.Info("подтверждение", "partition", success.Partition, "offset", success.Offset)
			case fail := <-producer.Errors():
				slog.Error("ошибка отправки", "error", fail.Err)
			}
		}
	}()

	// Все сообщения кладутся в канал Input() без ожидания подтверждения
	// Sarama внутри батчит их и отправляет на брокер
	start := time.Now()
	for i := range messageCount {
		msg := &sarama.ProducerMessage{
			Topic: topicName,
			Value: sarama.StringEncoder(fmt.Sprintf("async-%d: %s", i+1, gofakeit.StreetName())),
		}
		producer.Input() <- msg
		slog.Info("отправлено в канал", "message_num", i+1)
	}
	slog.Info("все сообщения отправлены в канал", "elapsed", time.Since(start))

	// Ждём, пока все подтверждения будут обработаны
	wg.Wait()
	slog.Info("все подтверждения получены", "count", messageCount, "elapsed", time.Since(start))

	producer.AsyncClose()
}

// newAsyncProducer создаёт асинхронный Kafka-продюсер с подтверждением от всех реплик
// Return.Successes и Return.Errors включены — без этого нельзя узнать, записалось ли сообщение
func newAsyncProducer(brokerList []string) (sarama.AsyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	producer, err := sarama.NewAsyncProducer(brokerList, config)
	if err != nil {
		return nil, err
	}

	return producer, nil
}
