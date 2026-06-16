package v1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/shared/pkg/proto/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/api/converter"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/errors"
)

func (a *api) Get(ctx context.Context, req *ufov1.GetRequest) (*ufov1.GetResponse, error) {
	sighting, err := a.ufoService.Get(ctx, req.GetUuid())
	if err != nil {
		if errors.Is(err, errs.ErrSightingNotFound) {
			return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
		}
		return nil, err
	}

	return &ufov1.GetResponse{
		Sighting: converter.SightingToDTO(sighting),
	}, nil
}
