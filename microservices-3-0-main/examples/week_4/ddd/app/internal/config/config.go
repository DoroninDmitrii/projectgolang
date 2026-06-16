package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

var appConfig *config

// config хранит конфигурацию приложения, загружаемую из переменных окружения
type config struct {
	HTTP   httpConfig   `yaml:"http"`
	Logger loggerConfig `yaml:"logger"`
	PG     pgConfig     `yaml:"pg"`
}

// MustLoad загружает конфигурацию из переменных окружения
// Паникует, если загрузка не удалась — приложение не может работать без конфига
func MustLoad() {
	var cfg config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		panic(fmt.Sprintf("не удалось загрузить конфиг: %v", err))
	}

	appConfig = &cfg
}

// AppConfig возвращает загруженную конфигурацию приложения
func AppConfig() *config {
	return appConfig
}
