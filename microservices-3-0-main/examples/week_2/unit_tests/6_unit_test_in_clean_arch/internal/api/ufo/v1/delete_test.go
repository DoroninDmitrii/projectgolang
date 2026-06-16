package v1

import (
	"github.com/brianvoe/gofakeit/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/errors"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/pkg/proto/ufo/v1"
)

func (s *APISuite) TestDeleteSuccess() {
	var (
		uuid = gofakeit.UUID()

		request = &ufov1.DeleteRequest{
			Uuid: uuid,
		}
	)

	s.ufoService.EXPECT().Delete(s.ctx, uuid).Return(nil)

	res, err := s.api.Delete(s.ctx, request)
	s.Require().NoError(err)
	s.Require().NotNil(res)
}

func (s *APISuite) TestDeleteEmptyUUID() {
	request := &ufov1.DeleteRequest{
		Uuid: "",
	}

	res, err := s.api.Delete(s.ctx, request)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *APISuite) TestDeleteNotFound() {
	var (
		uuid = gofakeit.UUID()

		request = &ufov1.DeleteRequest{
			Uuid: uuid,
		}
	)

	s.ufoService.EXPECT().Delete(s.ctx, uuid).Return(errs.ErrSightingNotFound)

	res, err := s.api.Delete(s.ctx, request)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *APISuite) TestDeleteServiceError() {
	var (
		serviceErr = gofakeit.Error()
		uuid       = gofakeit.UUID()

		request = &ufov1.DeleteRequest{
			Uuid: uuid,
		}
	)

	s.ufoService.EXPECT().Delete(s.ctx, uuid).Return(serviceErr)

	res, err := s.api.Delete(s.ctx, request)
	s.Require().Error(err)
	s.Require().Nil(res)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.Internal, st.Code())
}
