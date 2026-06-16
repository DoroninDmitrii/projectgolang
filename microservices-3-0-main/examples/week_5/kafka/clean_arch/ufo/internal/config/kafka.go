package config

type kafkaConfig struct {
	Brokers []string `env:"KAFKA_BROKERS" env-default:"localhost:9092" env-separator:","`
}
