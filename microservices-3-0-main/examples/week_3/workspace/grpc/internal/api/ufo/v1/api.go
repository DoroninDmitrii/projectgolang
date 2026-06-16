package v1

import (
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/proto/ufo/v1"
)

type api struct {
	ufov1.UnimplementedUFOServiceServer

	ufoRepository UFORepository
}

func NewAPI(ufoRepository UFORepository) *api {
	return &api{
		ufoRepository: ufoRepository,
	}
}
