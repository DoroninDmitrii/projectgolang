package ufo

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/service/ufo/mocks"
)

// Паттерн: Parallel Table-Driven Tests (Service Layer — Get)
//
// См. комментарии в create_test.go для подробного объяснения паттерна

func TestGet(t *testing.T) {
	t.Parallel()

	type args struct {
		uuid string
	}

	type expected struct {
		sighting model.Sighting
		err      error
	}

	var (
		ctx             = context.Background()
		uuid            = gofakeit.UUID()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		observedAt      = time.Now()
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)
		createdAt       = time.Now()
		updatedAt       = time.Now()

		repoErr = gofakeit.Error()

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

	tests := []struct {
		name      string
		args      args
		setupMock func(repo *mocks.UFORepository)
		expected  expected
	}{
		{
			name: "успешное получение наблюдения",
			args: args{uuid: uuid},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Get(ctx, uuid).Return(sighting, nil)
			},
			expected: expected{sighting: sighting, err: nil},
		},
		{
			name: "ошибка репозитория при получении",
			args: args{uuid: uuid},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Get(ctx, uuid).Return(model.Sighting{}, repoErr)
			},
			expected: expected{sighting: model.Sighting{}, err: repoErr},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ufoRepository := mocks.NewUFORepository(t)
			tc.setupMock(ufoRepository)

			svc := NewService(ufoRepository)
			res, err := svc.Get(ctx, tc.args.uuid)

			if tc.expected.err != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expected.err)
				assert.Equal(t, model.Sighting{}, res)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected.sighting, res)
			}
		})
	}
}
