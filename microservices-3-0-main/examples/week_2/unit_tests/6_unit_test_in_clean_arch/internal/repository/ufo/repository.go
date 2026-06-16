package ufo

import (
	"sync"

	repoModel "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/repository/model"
)

type repository struct {
	mu   sync.RWMutex
	data map[string]repoModel.Sighting
}

func NewRepository() *repository {
	return &repository{
		data: make(map[string]repoModel.Sighting),
	}
}
