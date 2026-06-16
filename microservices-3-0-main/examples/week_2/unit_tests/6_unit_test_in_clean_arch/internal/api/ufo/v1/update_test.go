package v1

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/api/converter"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/errors"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/pkg/proto/ufo/v1"
)

func (s *APISuite) TestUpdateSuccess() {
	var (
		uuid           = gofakeit.UUID()
		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()
		newObservedAt  = time.Now()

		req = &ufov1.UpdateRequest{
			Uuid:        uuid,
			ObservedAt:  timestamppb.New(newObservedAt),
			Description: wrapperspb.String(newDescription),
			Color:       wrapperspb.String(newColor),
		}

		expectedModelUpdateInfo = converter.UpdateRequestToInput(req)
	)

	s.ufoService.EXPECT().Update(s.ctx, uuid, expectedModelUpdateInfo).Return(nil)

	res, err := s.api.Update(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(res)
}

func (s *APISuite) TestUpdateEmptyUUID() {
	var (
		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")

		req = &ufov1.UpdateRequest{
			Uuid:        "",
			Description: wrapperspb.String(newDescription),
		}
	)

	res, err := s.api.Update(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *APISuite) TestUpdateNilInfo() {
	var (
		uuid = gofakeit.UUID()

		req = &ufov1.UpdateRequest{
			Uuid: uuid,
		}

		expectedModelUpdateInfo = converter.UpdateRequestToInput(req)
	)

	// Пустой update — no-op, сервис вызывается и возвращает nil
	s.ufoService.EXPECT().Update(s.ctx, uuid, expectedModelUpdateInfo).Return(nil)

	res, err := s.api.Update(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(res)
}

func (s *APISuite) TestUpdateNotFound() {
	var (
		uuid           = gofakeit.UUID()
		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()

		req = &ufov1.UpdateRequest{
			Uuid:        uuid,
			Description: wrapperspb.String(newDescription),
			Color:       wrapperspb.String(newColor),
		}

		expectedModelUpdateInfo = converter.UpdateRequestToInput(req)
	)

	s.ufoService.EXPECT().Update(s.ctx, uuid, expectedModelUpdateInfo).Return(errs.ErrSightingNotFound)

	res, err := s.api.Update(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *APISuite) TestUpdateServiceError() {
	var (
		serviceErr     = gofakeit.Error()
		uuid           = gofakeit.UUID()
		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()

		req = &ufov1.UpdateRequest{
			Uuid:        uuid,
			Description: wrapperspb.String(newDescription),
			Color:       wrapperspb.String(newColor),
		}

		expectedModelUpdateInfo = converter.UpdateRequestToInput(req)
	)

	s.ufoService.EXPECT().Update(s.ctx, uuid, expectedModelUpdateInfo).Return(serviceErr)

	res, err := s.api.Update(s.ctx, req)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.Internal, st.Code())
}
