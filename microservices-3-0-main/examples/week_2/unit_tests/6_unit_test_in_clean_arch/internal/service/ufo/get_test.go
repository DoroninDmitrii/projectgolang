package ufo

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
)

func (s *ServiceSuite) TestGetSuccess() {
	var (
		uuid            = gofakeit.UUID()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		observedAt      = time.Now()
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)
		createdAt       = time.Now()
		updatedAt       = time.Now()

		sighting = model.Sighting{
			Uuid:            uuid,
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
			Credibility:     "high",
			CreatedAt:       createdAt,
			UpdatedAt:       &updatedAt,
			DeletedAt:       nil,
		}
	)

	s.ufoRepository.EXPECT().Get(s.ctx, uuid).Return(sighting, nil)

	res, err := s.service.Get(s.ctx, uuid)
	s.NoError(err)
	s.Equal(sighting, res)
}

func (s *ServiceSuite) TestGetRepoError() {
	var (
		repoErr = gofakeit.Error()
		uuid    = gofakeit.UUID()
	)

	s.ufoRepository.EXPECT().Get(s.ctx, uuid).Return(model.Sighting{}, repoErr)

	res, err := s.service.Get(s.ctx, uuid)
	s.Error(err)
	s.ErrorIs(err, repoErr)
	s.Equal(model.Sighting{}, res)
}
