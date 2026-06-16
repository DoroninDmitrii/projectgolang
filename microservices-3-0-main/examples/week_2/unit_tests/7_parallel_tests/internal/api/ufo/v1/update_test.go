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
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/api/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/api/ufo/v1/mocks"
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/errors"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/pkg/proto/ufo/v1"
)

// Паттерн: Parallel Table-Driven Tests (API Layer — Update)
//
// См. комментарии в create_test.go для подробного объяснения паттерна

func TestUpdate(t *testing.T) {
	t.Parallel()

	type args struct {
		req *ufov1.UpdateRequest
	}

	type expected struct {
		notNilRes bool
		errCheck  func(t *testing.T, err error)
	}

	var (
		ctx            = context.Background()
		uuid           = gofakeit.UUID()
		newDescription = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		newColor       = gofakeit.Color()
		newObservedAt  = time.Now()

		serviceErr = gofakeit.Error()

		protoUpdateReq = &ufov1.UpdateRequest{
			Uuid:        uuid,
			ObservedAt:  timestamppb.New(newObservedAt),
			Description: wrapperspb.String(newDescription),
			Color:       wrapperspb.String(newColor),
		}

		expectedModelUpdateInfo = converter.UpdateRequestToInput(protoUpdateReq)
	)

	tests := []struct {
		name      string
		args      args
		setupMock func(svc *mocks.UFOService)
		expected  expected
	}{
		{
			name: "успешное обновление наблюдения",
			args: args{
				req: protoUpdateReq,
			},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Update(ctx, uuid, expectedModelUpdateInfo).Return(nil)
			},
			expected: expected{
				notNilRes: true,
				errCheck: func(t *testing.T, err error) {
					require.NoError(t, err)
				},
			},
		},
		{
			name: "пустой UUID — codes.InvalidArgument",
			args: args{
				req: &ufov1.UpdateRequest{
					Uuid:        "",
					Description: wrapperspb.String(newDescription),
				},
			},
			setupMock: func(_ *mocks.UFOService) {
				// Мок не настраиваем — валидация до вызова сервиса
			},
			expected: expected{
				notNilRes: false,
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
			args: args{
				req: protoUpdateReq,
			},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Update(ctx, uuid, expectedModelUpdateInfo).Return(errs.ErrSightingNotFound)
			},
			expected: expected{
				notNilRes: false,
				errCheck: func(t *testing.T, err error) {
					require.Error(t, err)
					st, ok := status.FromError(err)
					require.True(t, ok)
					assert.Equal(t, codes.NotFound, st.Code())
				},
			},
		},
		{
			name: "ошибка сервиса при обновлении — codes.Internal",
			args: args{
				req: protoUpdateReq,
			},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Update(ctx, uuid, expectedModelUpdateInfo).Return(serviceErr)
			},
			expected: expected{
				notNilRes: false,
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
			res, err := a.Update(ctx, tc.args.req)

			tc.expected.errCheck(t, err)

			if tc.expected.notNilRes {
				require.NotNil(t, res)
			} else {
				assert.Nil(t, res)
			}
		})
	}
}
