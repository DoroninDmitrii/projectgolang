package config

import "net"

type grpcConfig struct {
	Host string `yaml:"host" env:"GRPC_HOST" env-default:"localhost"`
	Port string `yaml:"port" env:"GRPC_PORT" env-default:"50051"`
}

func (c *grpcConfig) Address() string {
	return net.JoinHostPort(c.Host, c.Port)
}
