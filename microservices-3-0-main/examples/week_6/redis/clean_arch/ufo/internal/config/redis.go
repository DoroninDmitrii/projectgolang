package config

import (
	"net"
	"time"
)

type redisConfig struct {
	Host              string        `yaml:"host"               env:"REDIS_HOST"               env-default:"localhost"`
	Port              string        `yaml:"port"               env:"REDIS_PORT"               env-default:"6379"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout" env:"REDIS_CONNECTION_TIMEOUT" env-default:"10s"`
	MaxIdle           int           `yaml:"max_idle"           env:"REDIS_MAX_IDLE"           env-default:"10"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"       env:"REDIS_IDLE_TIMEOUT"       env-default:"10s"`
	CacheTTL          time.Duration `yaml:"cache_ttl"          env:"REDIS_CACHE_TTL"          env-default:"1h"`
}

func (c *redisConfig) Address() string {
	return net.JoinHostPort(c.Host, c.Port)
}
