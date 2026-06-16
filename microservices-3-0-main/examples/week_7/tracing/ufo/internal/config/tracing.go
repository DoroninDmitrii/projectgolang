package config

import (
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/platform/pkg/tracing"
)

type tracingConfig struct {
	CollectorEndpoint string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT" env-default:"localhost:4317"`
	ServiceName       string  `env:"OTEL_SERVICE_NAME"            env-default:"ufo"`
	Environment       string  `env:"OTEL_ENVIRONMENT"             env-default:"development"`
	ServiceVersion    string  `env:"OTEL_SERVICE_VERSION"          env-default:"0.1.0"`
	SamplingRatio     float64 `env:"OTEL_SAMPLING_RATIO"           env-default:"1.0"`
}

// TracingConfig конвертирует в tracing.Config для передачи в platform-пакет
func (c *tracingConfig) TracingConfig() tracing.Config {
	return tracing.Config{
		CollectorEndpoint: c.CollectorEndpoint,
		ServiceName:       c.ServiceName,
		Environment:       c.Environment,
		ServiceVersion:    c.ServiceVersion,
		SamplingRatio:     c.SamplingRatio,
	}
}
