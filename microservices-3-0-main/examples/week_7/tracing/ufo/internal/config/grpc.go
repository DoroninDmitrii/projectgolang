package config

import "net"

type grpcConfig struct {
	Host         string `env:"GRPC_HOST" env-default:"localhost"`
	Port         string `env:"GRPC_PORT" env-default:"50051"`
	AnalysisHost string `env:"ANALYSIS_GRPC_HOST" env-default:"localhost"`
	AnalysisPort string `env:"ANALYSIS_GRPC_PORT" env-default:"50052"`
}

// Address возвращает адрес gRPC-сервера UFO
func (c *grpcConfig) Address() string {
	return net.JoinHostPort(c.Host, c.Port)
}

// AnalysisAddress возвращает адрес gRPC-сервера Analysis
func (c *grpcConfig) AnalysisAddress() string {
	return net.JoinHostPort(c.AnalysisHost, c.AnalysisPort)
}
