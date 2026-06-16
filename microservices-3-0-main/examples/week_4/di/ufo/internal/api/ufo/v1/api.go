package v1

import (
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/shared/pkg/proto/ufo/v1"
)

// api реализует gRPC-обработчики сервиса наблюдений НЛО
type api struct {
	ufov1.UnimplementedUFOServiceServer

	ufoService UFOService
}

// New создаёт API-обработчик наблюдений НЛО
func New(ufoService UFOService) *api {
	return &api{
		ufoService: ufoService,
	}
}
