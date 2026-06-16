package v1

import (
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/config/shared/pkg/proto/ufo/v1"
)

type api struct {
	ufov1.UnimplementedUFOServiceServer

	ufoService UFOService
}

func NewAPI(ufoService UFOService) *api {
	return &api{
		ufoService: ufoService,
	}
}
