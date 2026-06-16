package v1

import (
	"context"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/api/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/api/ufo/v1/mocks"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/model"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/pkg/proto/ufo/v1"
)

// Паттерн: Parallel Table-Driven Tests (API Layer — Get)
//
// errCheck func(t *testing.T, err error) позволяет каждому кейсу определить
// свою логику проверки ошибок, включая gRPC-коды (см. п. 6 в create_test.go)
//
// См. комментарии в create_test.go для подробного объяснения паттерна

func TestGet(t *testing.T) {
	t.Parallel()

	type args struct {
		req *ufov1.GetRequest
	}

	type expected struct {
		res      *ufov1.GetResponse
		errCheck func(t *testing.T, err error)
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

		serviceErr = gofakeit.Error()

		req = &ufov1.GetRequest{
			Uuid: uuid,
		}

		modelSighting = model.Sighting{
			Uuid:            uuid,
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
			Credibility:     "high",
			CreatedAt:       createdAt,
		}

		expectedProtoSighting = converter.SightingToDTO(modelSighting)
	)

	tests := []struct {
		name      string
		args      args
		setupMock func(svc *mocks.UFOService)
		expected  expected
	}{
		{
			name: "успешное получение наблюдения",
			args: args{req: req},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Get(ctx, uuid).Return(modelSighting, nil)
			},
			expected: expected{
				res: &ufov1.GetResponse{Sighting: expectedProtoSighting},
				errCheck: func(t *testing.T, err error) {
					require.NoError(t, err)
				},
			},
		},
		{
			name: "пустой UUID — codes.InvalidArgument",
			args: args{
				req: &ufov1.GetRequest{
					Uuid: "",
				},
			},
			setupMock: func(_ *mocks.UFOService) {
				// Мок не настраиваем — валидация до вызова сервиса
			},
			expected: expected{
				res: nil,
				errCheck: func(t *testing.T, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					assert.Equal(t, codes.InvalidArgument, st.Code())
				},
			},
		},
		{
			name: "наблюдение не найдено — codes.NotFound",
			args: args{req: req},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Get(ctx, uuid).Return(model.Sighting{}, errs.ErrSightingNotFound)
			},
			expected: expected{
				res: nil,
				errCheck: func(t *testing.T, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					assert.Equal(t, codes.NotFound, st.Code())
				},
			},
		},
		{
			name: "ошибка сервиса при получении — codes.Internal",
			args: args{req: req},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Get(ctx, uuid).Return(model.Sighting{}, serviceErr)
			},
			expected: expected{
				res: nil,
				errCheck: func(t *testing.T, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					assert.Equal(t, codes.Internal, st.Code())
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ufoService := mocks.NewUFOService(t)
			tc.setupMock(ufoService)

			a := NewAPI(ufoService)
			res, err := a.Get(ctx, tc.args.req)

			tc.expected.errCheck(t, err)

			if tc.expected.res != nil {
				require.NotNil(t, res)
				assert.Equal(t, expectedProtoSighting.GetUuid(), res.GetSighting().GetUuid())
				assert.Equal(t, expectedProtoSighting.GetLocation(), res.GetSighting().GetLocation())
				assert.Equal(t, expectedProtoSighting.GetDescription(), res.GetSighting().GetDescription())
			} else {
				assert.Nil(t, res)
			}
		})
	}
}
