package config

import "github.com/IBM/sarama"

type ufoRecordedProducerConfig struct {
	TopicName string `env:"UFO_RECORDED_TOPIC_NAME" env-default:"ufo.recorded"`
}

func (c *ufoRecordedProducerConfig) Topic() string {
	return c.TopicName
}

func (c *ufoRecordedProducerConfig) SaramaConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V4_0_0_0
	cfg.Producer.Return.Successes = true

	return cfg
}
