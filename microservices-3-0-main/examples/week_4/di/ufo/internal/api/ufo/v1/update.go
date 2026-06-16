package v1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/shared/pkg/proto/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/api/converter"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/di/ufo/internal/errors"
)

func (a *api) Update(ctx context.Context, req *ufov1.UpdateRequest) (*ufov1.UpdateResponse, error) {
	if req.GetUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "uuid обязателен")
	}

	err := a.ufoService.Update(ctx, req.GetUuid(), converter.UpdateRequestToInput(req))
	if err != nil {
		if errors.Is(err, errs.ErrSightingNotFound) {
			return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
		}
		return nil, err
	}

	return &ufov1.UpdateResponse{}, nil
}
