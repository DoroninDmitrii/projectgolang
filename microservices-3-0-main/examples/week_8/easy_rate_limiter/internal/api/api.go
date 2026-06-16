// Package api реализует gRPC и HTTP хендлеры UFO-сервиса
// Бизнес-логика общая — оба транспорта работают с одним in-memory хранилищем
package api

import (
	"sync"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/pkg/proto/ufo/v1"
)

type ufo struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// api реализует gRPC UFOServiceServer и предоставляет HTTP хендлеры
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
