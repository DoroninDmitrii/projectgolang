package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

var appConfig *config

// config — корневая конфигурация приложения
type config struct {
	Logger  loggerConfig  `env-prefix:""`
	GRPC    grpcConfig    `env-prefix:""`
	PG      pgConfig      `env-prefix:""`
	Tracing tracingConfig `env-prefix:""`
}

// MustLoad загружает конфигурацию из переменных окружения
// Паникует при ошибке — без конфигурации приложение не может работать
func MustLoad() {
	var cfg config

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		panic(fmt.Sprintf("не удалось загрузить конфиг: %v", err))
	}

	appConfig = &cfg
}

// AppConfig возвращает загруженную конфигурацию
func AppConfig() *config {
	return appConfig
}
