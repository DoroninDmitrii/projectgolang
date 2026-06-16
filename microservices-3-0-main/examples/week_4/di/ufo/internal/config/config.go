package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

var appConfig *config

type config struct {
	GRPC   grpcConfig   `yaml:"grpc"`
	Logger loggerConfig `yaml:"logger"`
	PG     pgConfig     `yaml:"pg"`
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
