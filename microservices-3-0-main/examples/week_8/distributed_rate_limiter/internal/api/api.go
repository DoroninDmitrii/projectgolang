// Package api реализует gRPC хендлеры UFO-сервиса
package api

import (
	"sync"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/distributed_rate_limiter/pkg/proto/ufo/v1"
)

type ufo struct {
	ID      int64
	Title   string
	Content string
}

// api реализует gRPC UFOServiceServer
type api struct {
	ufov1.UnimplementedUFOServiceServer
	mu        sync.RWMutex
	sightings map[int64]*ufo
	nextID    int64
}

// NewAPI создаёт новый экземпляр API
func NewAPI() *api {
	return &api{
		sightings: make(map[int64]*ufo),
		nextID:    1,
	}
}
