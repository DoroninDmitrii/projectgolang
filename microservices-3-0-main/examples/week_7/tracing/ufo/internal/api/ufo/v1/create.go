package v1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/ufo/internal/api/converter"
)

func (a *api) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	uuid, err := a.ufoService.Create(ctx, converter.CreateRequestToInput(req))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "не удалось создать наблюдение: %v", err)
	}

	return &ufov1.CreateResponse{
		Uuid: uuid,
	}, nil
}
