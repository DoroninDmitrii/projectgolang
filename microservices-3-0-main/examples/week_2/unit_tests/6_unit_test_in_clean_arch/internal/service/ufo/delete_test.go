package ufo

import (
	"github.com/brianvoe/gofakeit/v7"
)

func (s *ServiceSuite) TestDeleteSuccess() {
	uuid := gofakeit.UUID()

	s.ufoRepository.EXPECT().Delete(s.ctx, uuid).Return(nil)

	err := s.service.Delete(s.ctx, uuid)
	s.NoError(err)
}

func (s *ServiceSuite) TestDeleteRepoError() {
	var (
		repoErr = gofakeit.Error()
		uuid    = gofakeit.UUID()
	)

	s.ufoRepository.EXPECT().Delete(s.ctx, uuid).Return(repoErr)

	err := s.service.Delete(s.ctx, uuid)
	s.Error(err)
	s.ErrorIs(err, repoErr)
}
