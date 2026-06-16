package ufo

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/service/input"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/service/ufo/mocks"
)

// Паттерн: Parallel Table-Driven Tests (Service Layer — Update)
//
// Update теперь содержит бизнес-логику: Get → applyUpdate → пересчёт credibility → Update
// Тесты проверяют корректность partial update, пересчёт credibility и propagation ошибок
//
// См. комментарии в create_test.go для подробного объяснения паттерна

func TestUpdate(t *testing.T) {
	t.Parallel()

	type args struct {
		uuid       string
		updateInfo input.UpdateSightingInput
	}

	type expected struct {
		err error
	}

	var (
		ctx         = context.Background()
		uuid        = gofakeit.UUID()
		observedAt  = time.Now()
		location    = gofakeit.City()
		description = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		color       = gofakeit.Color()
		sound       = true
		duration    = int32(60)

		repoErr = gofakeit.Error()

		existingFull = model.Sighting{
			Uuid:            uuid,
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &duration,
			Credibility:     "high",
			CreatedAt:       time.Now(),
		}

		existingPartial = model.Sighting{
			Uuid:        uuid,
			ObservedAt:  &observedAt,
			Location:    location,
			Credibility: "low",
			CreatedAt:   time.Now(),
		}

		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()
		newObservedAt  = time.Now()
		newSound       = false
		newDuration    = int32(120)
		newLocation    = gofakeit.City()

		fullUpdateInfo = input.UpdateSightingInput{
			ObservedAt:      &newObservedAt,
			Location:        &newLocation,
			Description:     &newDescription,
			Color:           &newColor,
			Sound:           &newSound,
			DurationSeconds: &newDuration,
		}

		partialUpdateInfo = input.UpdateSightingInput{
			Description: &newDescription,
			Color:       &newColor,
		}
	)

	tests := []struct {
		name      string
		args      args
		setupMock func(repo *mocks.UFORepository)
		expected  expected
	}{
		{
			name: "успешное обновление наблюдения (полное обновление)",
			args: args{uuid: uuid, updateInfo: fullUpdateInfo},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Get(ctx, uuid).Return(existingFull, nil)
				repo.EXPECT().Update(ctx, mock.MatchedBy(func(sg model.Sighting) bool {
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
			},
			expected: expected{err: nil},
		},
		{
			name: "успешное обновление наблюдения (partial update — credibility пересчитан)",
			args: args{uuid: uuid, updateInfo: partialUpdateInfo},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Get(ctx, uuid).Return(existingPartial, nil)
				// existing: ObservedAt + Location (2 поля) + update: Description + Color → 4 поля → "medium"
				repo.EXPECT().Update(ctx, mock.MatchedBy(func(sg model.Sighting) bool {
					return sg.Uuid == uuid &&
						sg.Location == location && // не менялось
						sg.Description == newDescription && // обновлено
						sg.Color != nil && *sg.Color == newColor && // обновлено
						sg.ObservedAt != nil && // не менялось
						sg.Credibility == "medium" && // 4 поля → medium
						sg.UpdatedAt != nil
				})).Return(nil)
			},
			expected: expected{err: nil},
		},
		{
			name: "ошибка репозитория при получении (Get)",
			args: args{uuid: uuid, updateInfo: partialUpdateInfo},
			setupMock: func(repo *mocks.UFORepository) {
				// Get фейлится → Update НЕ вызывается
				repo.EXPECT().Get(ctx, uuid).Return(model.Sighting{}, repoErr)
			},
			expected: expected{err: repoErr},
		},
		{
			name: "ошибка репозитория при обновлении (Update)",
			args: args{uuid: uuid, updateInfo: partialUpdateInfo},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Get(ctx, uuid).Return(existingPartial, nil)
				repo.EXPECT().Update(ctx, mock.MatchedBy(func(sg model.Sighting) bool {
					return sg.Uuid == uuid
				})).Return(repoErr)
			},
			expected: expected{err: repoErr},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ufoRepository := mocks.NewUFORepository(t)
			tc.setupMock(ufoRepository)

			svc := NewService(ufoRepository)
			err := svc.Update(ctx, tc.args.uuid, tc.args.updateInfo)

			if tc.expected.err != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expected.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
