package v1

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/api/converter"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/pkg/proto/ufo/v1"
)

func (s *APISuite) TestCreateSuccess() {
	var (
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		observedAt      = time.Now()
		color           = gofakeit.Color()
		sound           = gofakeit.Bool()
		durationSeconds = int32(60)

		expectedUUID = gofakeit.UUID()

		req = &ufov1.CreateRequest{
			ObservedAt:      timestamppb.New(observedAt),
			Location:        location,
			Description:     description,
			Color:           wrapperspb.String(color),
			Sound:           wrapperspb.Bool(sound),
			DurationSeconds: wrapperspb.Int32(durationSeconds),
		}

		expectedModelInfo = converter.CreateRequestToInput(req)
	)

	s.ufoService.EXPECT().Create(s.ctx, expectedModelInfo).Return(expectedUUID, nil)

	res, err := s.api.Create(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Require().Equal(expectedUUID, res.GetUuid())
}

func (s *APISuite) TestCreateNilInfo() {
	req := &ufov1.CreateRequest{
		Location: "",
	}

	res, err := s.api.Create(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *APISuite) TestCreateServiceError() {
	var (
		serviceErr      = gofakeit.Error()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		observedAt      = time.Now()
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)

		req = &ufov1.CreateRequest{
			ObservedAt:      timestamppb.New(observedAt),
			Location:        location,
			Description:     description,
			Color:           wrapperspb.String(color),
			Sound:           wrapperspb.Bool(sound),
			DurationSeconds: wrapperspb.Int32(durationSeconds),
		}

		expectedModelInfo = converter.CreateRequestToInput(req)
	)

	s.ufoService.EXPECT().Create(s.ctx, expectedModelInfo).Return("", serviceErr)

	res, err := s.api.Create(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.Internal, st.Code())
}
