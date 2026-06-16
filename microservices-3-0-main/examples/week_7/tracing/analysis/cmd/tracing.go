package main

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/platform/pkg/tracing"
)

const (
	collectorEndpoint = "localhost:4317"
	serviceName       = "analysis-service"
	environment       = "development"
	serviceVersion    = "1.0.0"
)

// initTracing инициализирует OpenTelemetry трейсер и возвращает функцию завершения
func initTracing(ctx context.Context) (func(context.Context) error, error) {
	cfg := tracing.Config{
		CollectorEndpoint: collectorEndpoint,
		ServiceName:       serviceName,
		Environment:       environment,
		ServiceVersion:    serviceVersion,
	}

	shutdown, err := tracing.InitTracer(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("инициализировать трейсер: %w", err)
	}

	return shutdown, nil
}
