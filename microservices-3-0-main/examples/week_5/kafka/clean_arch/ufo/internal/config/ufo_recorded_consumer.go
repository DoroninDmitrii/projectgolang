package config

import "github.com/IBM/sarama"

type ufoRecordedConsumerConfig struct {
	TopicName string `env:"UFO_RECORDED_TOPIC_NAME"        env-default:"ufo.recorded"`
	Group     string `env:"UFO_RECORDED_CONSUMER_GROUP_ID" env-default:"2"`
}

func (c *ufoRecordedConsumerConfig) Topic() string {
	return c.TopicName
}

func (c *ufoRecordedConsumerConfig) GroupID() string {
	return c.Group
}

func (c *ufoRecordedConsumerConfig) SaramaConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V4_0_0_0
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	return cfg
}
