package config

import "net"

type httpConfig struct {
	Host string `yaml:"host" env:"HTTP_HOST" env-default:"0.0.0.0"`
	Port string `yaml:"port" env:"HTTP_PORT" env-default:"8080"`
}

// Addr возвращает адрес для прослушивания HTTP-сервера
func (c *httpConfig) Addr() string {
	return net.JoinHostPort(c.Host, c.Port)
}
