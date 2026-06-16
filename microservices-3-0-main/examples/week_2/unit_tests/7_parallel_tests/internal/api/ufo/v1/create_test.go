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
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/pkg/proto/ufo/v1"
)

// Паттерн: Parallel Table-Driven Tests (API Layer — Create)
//
// Ключевые принципы:
//
//  1. t.Parallel() на уровне TestCreate — разрешает параллельный запуск
//     с другими top-level тестами (TestGet, TestUpdate, TestDelete)
//
//  2. t.Parallel() внутри t.Run() — подтесты запускаются параллельно
//     друг с другом
//
//  3. Мок создаётся ВНУТРИ каждого подтеста — это гарантирует изоляцию
//     Если бы мок был общим (как в suite), параллельные подтесты вызывали бы
//     data race при одновременном доступе к mock.ExpectedCalls
//
//  4. tc := tc — раньше (Go < 1.22) требовался захват переменной цикла,
//     так как переменная цикла переиспользовалась, и без захвата все горутины
//     подтестов ссылались бы на последний элемент слайса. Начиная с Go 1.22,
//     каждая итерация цикла создаёт свою переменную, поэтому tc := tc больше
//     не нужен. Мы его убрали
//
//  5. require vs assert — два пакета testify с разным поведением при провале:
//     - require.* вызывает t.FailNow() (→ runtime.Goexit()) — тест ОСТАНАВЛИВАЕТСЯ немедленно
//       Прерывается только текущий подтест (t.Run), остальные подтесты и тесты продолжат работу
//       Используем для предусловий (gates): если err != nil или res == nil,
//       дальнейшие проверки бессмысленны (будет nil-pointer panic)
//     - assert.* вызывает t.Fail() — тест ПРОДОЛЖАЕТСЯ, собирая все ошибки
//       Используем для проверки конкретных значений (checks): если одно поле неверное,
//       хотим увидеть ВСЕ расхождения, а не только первое
//     Правило: require — для «ворот» (gates), assert — для «проверок» (checks)
//
//  6. errCheck func(t *testing.T, err error) — позволяет каждому кейсу определить
//     свою логику проверки ошибок (gRPC-коды, ErrorIs и т.д.)

func TestCreate(t *testing.T) {
	t.Parallel()

	type args struct {
		req *ufov1.CreateRequest
	}

	type expected struct {
		res      *ufov1.CreateResponse
		errCheck func(t *testing.T, err error)
	}

	var (
		ctx             = context.Background()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		observedAt      = time.Now()
		color           = gofakeit.Color()
		sound           = gofakeit.Bool()
		durationSeconds = int32(60)

		expectedUUID = gofakeit.UUID()
		serviceErr   = gofakeit.Error()

		req = &ufov1.CreateRequest{
			ObservedAt:      timestamppb.New(observedAt),
			Location:        location,
			Description:     description,
			Color:           wrapperspb.String(color),
			Sound:           wrapperspb.Bool(sound),
			DurationSeconds: wrapperspb.Int32(durationSeconds),
		}

		expectedModelInfo = converter.CreateRequestToInput(req)
	)

	tests := []struct {
		name      string
		args      args
		setupMock func(svc *mocks.UFOService)
		expected  expected
	}{
		{
			name: "успешное создание наблюдения",
			args: args{req: req},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Create(ctx, expectedModelInfo).Return(expectedUUID, nil)
			},
			expected: expected{
				res: &ufov1.CreateResponse{Uuid: expectedUUID},
				errCheck: func(t *testing.T, err error) {
					require.NoError(t, err)
				},
			},
		},
		{
			name: "пустой location — codes.InvalidArgument",
			args: args{req: &ufov1.CreateRequest{Location: ""}},
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
			name: "ошибка сервиса при создании — codes.Internal",
			args: args{req: req},
			setupMock: func(svc *mocks.UFOService) {
				svc.EXPECT().Create(ctx, expectedModelInfo).Return("", serviceErr)
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

			// Каждый подтест создаёт СВОЙ мок — изоляция, без data race
			ufoService := mocks.NewUFOService(t)
			tc.setupMock(ufoService)

			a := NewAPI(ufoService)
			res, err := a.Create(ctx, tc.args.req)

			tc.expected.errCheck(t, err)

			// require vs assert — см. п. 5 в заголовке файла
			if tc.expected.res != nil {
				require.NotNil(t, res)
				assert.Equal(t, tc.expected.res.GetUuid(), res.GetUuid())
			} else {
				assert.Nil(t, res)
			}
		})
	}
}
