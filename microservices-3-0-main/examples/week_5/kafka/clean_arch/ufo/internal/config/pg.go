package config

import "fmt"

type pgConfig struct {
	Host     string `env:"POSTGRES_HOST"     env-default:"localhost"`
	Port     string `env:"POSTGRES_PORT"     env-default:"5432"`
	Database string `env:"POSTGRES_DB"       env-default:"ufo"`
	User     string `env:"POSTGRES_USER"     env-default:"ufo_admin"`
	Password string `env:"POSTGRES_PASSWORD" env-default:"ufo_secret"`
	SSLMode  string `env:"POSTGRES_SSLMODE"  env-default:"disable"`
}

func (c *pgConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		c.Host, c.Port, c.Database, c.User, c.Password, c.SSLMode,
	)
}
