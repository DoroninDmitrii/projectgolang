package v1

import (
	"context"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/shared/pkg/proto/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/api/converter"
)

func (a *api) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	uuid, err := a.ufoService.Create(ctx, converter.CreateRequestToInput(req))
	if err != nil {
		return nil, err
	}

	return &ufov1.CreateResponse{
		Uuid: uuid,
	}, nil
}
