package ufo

import (
	"sync"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/repository/record"
)

type repository struct {
	mu   sync.RWMutex
	data map[string]record.Sighting
}

func NewRepository() *repository {
	return &repository{
		data: make(map[string]record.Sighting),
	}
}
