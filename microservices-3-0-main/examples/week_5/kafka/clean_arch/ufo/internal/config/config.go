package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

var appConfig *config

type config struct {
	Logger              loggerConfig
	GRPC                grpcConfig
	PG                  pgConfig
	Kafka               kafkaConfig
	UfoRecordedProducer ufoRecordedProducerConfig
	UfoRecordedConsumer ufoRecordedConsumerConfig
}

func MustLoad() {
	var cfg config

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		panic(fmt.Sprintf("не удалось загрузить конфиг: %v", err))
	}

	appConfig = &cfg
}

func AppConfig() *config {
	return appConfig
}
