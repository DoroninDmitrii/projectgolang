package v1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/api/converter"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/pkg/proto/ufo/v1"
)

func (a *api) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	if req.GetLocation() == "" {
		return nil, status.Error(codes.InvalidArgument, "location не может быть пустым")
	}

	uuid, err := a.ufoService.Create(ctx, converter.CreateRequestToInput(req))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ошибка создания наблюдения: %v", err)
	}

	return &ufov1.CreateResponse{
		Uuid: uuid,
	}, nil
}
