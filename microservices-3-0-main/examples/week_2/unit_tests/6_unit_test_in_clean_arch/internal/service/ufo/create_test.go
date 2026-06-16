package ufo

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/mock"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/service/input"
)

// matchesInput возвращает matcher, который сверяет 6 полей model.Sighting
// со значениями из input.CreateSightingInput (по значению, не по указателям).
func matchesInput(in input.CreateSightingInput) func(s model.Sighting) bool {
	return func(s model.Sighting) bool {
		return s.Uuid != "" &&
			!s.CreatedAt.IsZero() &&
			equalTimePtr(s.ObservedAt, in.ObservedAt) &&
			s.Location == in.Location &&
			s.Description == in.Description &&
			equalStringPtr(s.Color, in.Color) &&
			equalBoolPtr(s.Sound, in.Sound) &&
			equalInt32Ptr(s.DurationSeconds, in.DurationSeconds)
	}
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalInt32Ptr(a, b *int32) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *ServiceSuite) TestCreateSuccessHighCredibility() {
	var (
		observedAt      = time.Now()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)

		createInput = input.CreateSightingInput{
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
		}
	)

	// Все 6 полей заполнены → score == 6 → credibility == "high".
	// UUID и CreatedAt генерируются внутри Create, поэтому проверяем через mock.MatchedBy
	s.ufoRepository.EXPECT().Create(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
		return matchesInput(createInput)(sg) && sg.Credibility == "high"
	})).Return(nil)

	uuid, err := s.service.Create(s.ctx, createInput)
	s.Require().NoError(err)
	s.NotEmpty(uuid)
}

func (s *ServiceSuite) TestCreateSuccessMediumCredibility() {
	var (
		observedAt  = time.Now()
		location    = gofakeit.City()
		description = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")

		// 3 поля: ObservedAt, Location, Description → score == 3 → "medium"
		createInput = input.CreateSightingInput{
			ObservedAt:  &observedAt,
			Location:    location,
			Description: description,
		}
	)

	s.ufoRepository.EXPECT().Create(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
		return matchesInput(createInput)(sg) && sg.Credibility == "medium"
	})).Return(nil)

	uuid, err := s.service.Create(s.ctx, createInput)
	s.Require().NoError(err)
	s.NotEmpty(uuid)
}

func (s *ServiceSuite) TestCreateSuccessLowCredibility() {
	var (
		location = gofakeit.City()

		// 1 поле: Location → score == 1 → "low"
		createInput = input.CreateSightingInput{
			Location: location,
		}
	)

	s.ufoRepository.EXPECT().Create(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
		return matchesInput(createInput)(sg) && sg.Credibility == "low"
	})).Return(nil)

	uuid, err := s.service.Create(s.ctx, createInput)
	s.Require().NoError(err)
	s.NotEmpty(uuid)
}

func (s *ServiceSuite) TestCreateRepoError() {
	var (
		repoErr         = gofakeit.Error()
		observedAt      = time.Now()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)

		createInput = input.CreateSightingInput{
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
		}
	)

	s.ufoRepository.EXPECT().Create(s.ctx, mock.MatchedBy(matchesInput(createInput))).Return(repoErr)

	uuid, err := s.service.Create(s.ctx, createInput)
	s.Error(err)
	s.ErrorIs(err, repoErr)
	s.Empty(uuid)
}
