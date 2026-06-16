package config

import "fmt"

type pgConfig struct {
	Host     string `yaml:"host"     env:"POSTGRES_HOST"     env-default:"localhost"`
	Port     string `yaml:"port"     env:"POSTGRES_PORT"     env-default:"5432"`
	Database string `yaml:"database" env:"POSTGRES_DB"       env-default:"ufo"`
	User     string `yaml:"user"     env:"POSTGRES_USER"     env-default:"ufo_admin"`
	Password string `yaml:"password" env:"POSTGRES_PASSWORD" env-default:"ufo_secret"`
	SSLMode  string `yaml:"sslmode"  env:"POSTGRES_SSLMODE"  env-default:"disable"`
}

func (c *pgConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		c.Host, c.Port, c.Database, c.User, c.Password, c.SSLMode,
	)
}
