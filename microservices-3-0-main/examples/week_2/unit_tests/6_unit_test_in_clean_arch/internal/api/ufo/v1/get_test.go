package v1

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/api/converter"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/pkg/proto/ufo/v1"
)

func (s *APISuite) TestGetSuccess() {
	var (
		uuid            = gofakeit.UUID()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		observedAt      = time.Now()
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)
		createdAt       = time.Now()

		req = &ufov1.GetRequest{
			Uuid: uuid,
		}

		modelSighting = model.Sighting{
			Uuid:            uuid,
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
			Credibility:     "high",
			CreatedAt:       createdAt,
		}

		expectedProtoSighting = converter.SightingToDTO(modelSighting)
	)

	s.ufoService.EXPECT().Get(s.ctx, uuid).Return(modelSighting, nil)

	res, err := s.api.Get(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Require().Equal(expectedProtoSighting.Uuid, res.GetSighting().GetUuid())
	s.Require().Equal(expectedProtoSighting.Location, res.GetSighting().GetLocation())
	s.Require().Equal(expectedProtoSighting.Description, res.GetSighting().GetDescription())
}

func (s *APISuite) TestGetNotFound() {
	var (
		uuid = gofakeit.UUID()

		req = &ufov1.GetRequest{
			Uuid: uuid,
		}
	)

	s.ufoService.EXPECT().Get(s.ctx, uuid).Return(model.Sighting{}, errs.ErrSightingNotFound)

	res, err := s.api.Get(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *APISuite) TestGetEmptyUUID() {
	req := &ufov1.GetRequest{
		Uuid: "",
	}

	res, err := s.api.Get(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *APISuite) TestGetServiceError() {
	var (
		serviceErr = gofakeit.Error()
		uuid       = gofakeit.UUID()

		req = &ufov1.GetRequest{
			Uuid: uuid,
		}
	)

	s.ufoService.EXPECT().Get(s.ctx, uuid).Return(model.Sighting{}, serviceErr)

	res, err := s.api.Get(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.Internal, st.Code())
}
