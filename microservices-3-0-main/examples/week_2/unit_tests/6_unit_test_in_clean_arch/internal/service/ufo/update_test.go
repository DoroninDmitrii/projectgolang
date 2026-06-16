package ufo

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/mock"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/service/input"
)

func (s *ServiceSuite) TestUpdateSuccessFullUpdate() {
	var (
		uuid            = gofakeit.UUID()
		observedAt      = time.Now()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)

		existingSighting = model.Sighting{
			Uuid:            uuid,
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
			Credibility:     "high",
			CreatedAt:       time.Now(),
		}

		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()
		newObservedAt  = time.Now()
		newSound       = false
		newDuration    = int32(120)
		newLocation    = gofakeit.City()

		updateInfo = input.UpdateSightingInput{
			ObservedAt:      &newObservedAt,
			Location:        &newLocation,
			Description:     &newDescription,
			Color:           &newColor,
			Sound:           &newSound,
			DurationSeconds: &newDuration,
		}
	)

	// Шаг 1: Get возвращает existing sighting
	s.ufoRepository.EXPECT().Get(s.ctx, uuid).Return(existingSighting, nil)

	// Шаг 2: Update получает sighting с обновлёнными полями и пересчитанным credibility
	s.ufoRepository.EXPECT().Update(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
		return sg.Uuid == uuid &&
			sg.Location == newLocation &&
			sg.Description == newDescription &&
			sg.Color != nil && *sg.Color == newColor &&
			sg.Sound != nil && *sg.Sound == newSound &&
			sg.DurationSeconds != nil && *sg.DurationSeconds == newDuration &&
			sg.ObservedAt != nil &&
			sg.Credibility == "high" && // все 6 полей → high
			sg.UpdatedAt != nil && !sg.UpdatedAt.IsZero()
	})).Return(nil)

	err := s.service.Update(s.ctx, uuid, updateInfo)
	s.NoError(err)
}

func (s *ServiceSuite) TestUpdateSuccessPartialUpdate() {
	var (
		uuid       = gofakeit.UUID()
		observedAt = time.Now()
		location   = gofakeit.City()

		existingSighting = model.Sighting{
			Uuid:        uuid,
			ObservedAt:  &observedAt,
			Location:    location,
			Credibility: "low",
			CreatedAt:   time.Now(),
		}

		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()

		// Обновляем только 2 поля → Description + Color
		updateInfo = input.UpdateSightingInput{
			Description: &newDescription,
			Color:       &newColor,
		}
	)

	s.ufoRepository.EXPECT().Get(s.ctx, uuid).Return(existingSighting, nil)

	// После partial update: ObservedAt, Location (из existing) + Description, Color (из update) → 4 поля → "medium"
	s.ufoRepository.EXPECT().Update(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
		return sg.Uuid == uuid &&
			sg.Location == location && // не менялось
			sg.Description == newDescription && // обновлено
			sg.Color != nil && *sg.Color == newColor && // обновлено
			sg.ObservedAt != nil && // не менялось
			sg.Credibility == "medium" && // 4 поля → medium
			sg.UpdatedAt != nil
	})).Return(nil)

	err := s.service.Update(s.ctx, uuid, updateInfo)
	s.NoError(err)
}

func (s *ServiceSuite) TestUpdateGetError() {
	var (
		repoErr        = gofakeit.Error()
		uuid           = gofakeit.UUID()
		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")

		updateInfo = input.UpdateSightingInput{
			Description: &newDescription,
		}
	)

	// Get фейлится → Update НЕ вызывается
	s.ufoRepository.EXPECT().Get(s.ctx, uuid).Return(model.Sighting{}, repoErr)

	err := s.service.Update(s.ctx, uuid, updateInfo)
	s.Error(err)
	s.ErrorIs(err, repoErr)
}

func (s *ServiceSuite) TestUpdateRepoError() {
	var (
		repoErr  = gofakeit.Error()
		uuid     = gofakeit.UUID()
		location = gofakeit.City()

		existingSighting = model.Sighting{
			Uuid:        uuid,
			Location:    location,
			Credibility: "low",
			CreatedAt:   time.Now(),
		}

		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")

		updateInfo = input.UpdateSightingInput{
			Description: &newDescription,
		}
	)

	s.ufoRepository.EXPECT().Get(s.ctx, uuid).Return(existingSighting, nil)
	s.ufoRepository.EXPECT().Update(s.ctx, mock.MatchedBy(func(sg model.Sighting) bool {
		return sg.Uuid == uuid
	})).Return(repoErr)

	err := s.service.Update(s.ctx, uuid, updateInfo)
	s.Error(err)
	s.ErrorIs(err, repoErr)
}
