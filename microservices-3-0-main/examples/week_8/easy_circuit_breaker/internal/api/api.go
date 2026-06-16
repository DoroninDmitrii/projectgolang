// Package api реализует gRPC хендлеры UFO-сервиса с поддержкой симуляции сбоев
package api

import (
	"sync"
	"sync/atomic"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_circuit_breaker/pkg/proto/ufo/v1"
)

type ufo struct {
	ID      int64
	Title   string
	Content string
}

// api реализует gRPC UFOServiceServer с in-memory хранилищем и режимом симуляции сбоев
type api struct {
	ufov1.UnimplementedUFOServiceServer
	mu        sync.RWMutex
	sightings map[int64]*ufo
	nextID    int64
	failMode  atomic.Bool
}

// NewAPI создаёт новый экземпляр API
func NewAPI() *api {
	return &api{
		sightings: make(map[int64]*ufo),
		nextID:    1,
	}
}
