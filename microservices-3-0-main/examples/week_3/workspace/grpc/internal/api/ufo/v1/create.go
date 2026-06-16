package v1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/grpc/internal/api/converter"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/shared/pkg/proto/ufo/v1"
)

func (a *api) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	if req.GetLocation() == "" {
		return nil, status.Error(codes.InvalidArgument, "location не может быть пустым")
	}

	uuid, err := a.ufoRepository.Create(ctx, converter.CreateRequestToInput(req))
	if err != nil {
		return nil, err
	}

	return &ufov1.CreateResponse{
		Uuid: uuid,
	}, nil
}
