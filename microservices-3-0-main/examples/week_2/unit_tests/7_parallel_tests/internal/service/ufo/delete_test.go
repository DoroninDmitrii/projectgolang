package ufo

import (
	"context"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/service/ufo/mocks"
)

// Паттерн: Parallel Table-Driven Tests (Service Layer — Delete)
//
// См. комментарии в create_test.go для подробного объяснения паттерна

func TestDelete(t *testing.T) {
	t.Parallel()

	type args struct {
		uuid string
	}

	type expected struct {
		err error
	}

	var (
		ctx  = context.Background()
		uuid = gofakeit.UUID()

		repoErr = gofakeit.Error()
	)

	tests := []struct {
		name      string
		args      args
		setupMock func(repo *mocks.UFORepository)
		expected  expected
	}{
		{
			name: "успешное удаление наблюдения",
			args: args{uuid: uuid},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Delete(ctx, uuid).Return(nil)
			},
			expected: expected{err: nil},
		},
		{
			name: "ошибка репозитория при удалении",
			args: args{uuid: uuid},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Delete(ctx, uuid).Return(repoErr)
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
			err := svc.Delete(ctx, tc.args.uuid)

			if tc.expected.err != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expected.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
