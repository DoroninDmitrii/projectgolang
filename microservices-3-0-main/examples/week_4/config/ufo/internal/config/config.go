package config

import (
	"flag"
	"fmt"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type config struct {
	GRPC grpcConfig `yaml:"grpc"`
	PG   pgConfig   `yaml:"pg"`
}

const defaultConfigPath = "config.local.yaml"

// ResolveConfigPath определяет путь к конфиг-файлу по цепочке приоритетов:
// флаг -config > env CONFIG_PATH > "config.local.yaml".
func ResolveConfigPath() string {
	var cfgFlag string
	flag.StringVar(&cfgFlag, "config", "", "путь к YAML-конфигу (например, config.staging.yaml)")
	flag.Parse()

	if cfgFlag != "" {
		return cfgFlag
	}

	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		return envPath
	}

	return defaultConfigPath
}

func Load(path string) (*config, error) {
	var cfg config

	if path != "" {
		// ReadConfig читает YAML-файл, а затем перетирает значения из env-переменных
		// Приоритет: env > yaml > env-default
		if err := cleanenv.ReadConfig(path, &cfg); err != nil {
			return nil, fmt.Errorf("не удалось загрузить конфиг из %q: %w", path, err)
		}

		return &cfg, nil
	}

	// Если путь не указан — читаем только из env-переменных
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("не удалось загрузить конфиг из env: %w", err)
	}

	return &cfg, nil
}
