package kafka

import "time"

// Header — заголовок Kafka-сообщения (ключ-значение)
// Kafka допускает дублирование ключей, поэтому используем слайс, а не map
type Header struct {
	Key   string
	Value []byte
}

// Message — универсальная обёртка над сообщением Kafka
type Message struct {
	Headers   []Header
	Timestamp time.Time

	Key       []byte
	Value     []byte
	Topic     string
	Partition int32
	Offset    int64
}
