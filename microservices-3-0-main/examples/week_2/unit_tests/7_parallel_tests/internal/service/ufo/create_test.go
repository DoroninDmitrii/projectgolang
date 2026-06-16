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

// Паттерн: Parallel Table-Driven Tests (Service Layer — Create)
//
// Ключевые принципы:
//
//  1. t.Parallel() на уровне TestCreate — разрешает параллельный запуск
//     с другими top-level тестами (TestGet, TestUpdate, TestDelete)
//
//  2. t.Parallel() внутри t.Run() — подтесты ("успех", "ошибка") запускаются
//     параллельно друг с другом
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
//  6. mock.MatchedBy — UUID и CreatedAt генерируются внутри Create, поэтому
//     нельзя передать точное значение model.Sighting в EXPECT().Create(...)
//     Вместо этого используем mock.MatchedBy(func(s model.Sighting) bool {...}),
//     который проверяет каждое поле отдельно: UUID != "", поля совпадают
//     с input, Credibility правильный, CreatedAt не нулевой

func TestCreate(t *testing.T) {
	t.Parallel()

	type args struct {
		in input.CreateSightingInput
	}

	type expected struct {
		err error
	}

	var (
		ctx             = context.Background()
		observedAt      = time.Now()
		location        = gofakeit.City()
		description     = gofakeit.LoremIpsumParagraph(3, 5, 5, " ")
		color           = gofakeit.Color()
		sound           = true
		durationSeconds = int32(60)

		repoErr = gofakeit.Error()

		// Все 6 полей → score == 6 → "high"
		fullInput = input.CreateSightingInput{
			ObservedAt:      &observedAt,
			Location:        location,
			Description:     description,
			Color:           &color,
			Sound:           &sound,
			DurationSeconds: &durationSeconds,
		}

		// 3 поля: ObservedAt, Location, Description → score == 3 → "medium"
		mediumInput = input.CreateSightingInput{
			ObservedAt:  &observedAt,
			Location:    location,
			Description: description,
		}

		// 1 поле: Location → score == 1 → "low"
		lowInput = input.CreateSightingInput{
			Location: location,
		}
	)

	// matchesInput проверяет, что Sighting содержит все поля из input.
	// Сравнение по значению, а не по указателям — экземпляры разные.
	matchesInput := func(in input.CreateSightingInput) func(s model.Sighting) bool {
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

	tests := []struct {
		name      string
		args      args
		setupMock func(repo *mocks.UFORepository)
		expected  expected
	}{
		{
			name: "успешное создание наблюдения (high credibility)",
			args: args{in: fullInput},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Create(ctx, mock.MatchedBy(func(s model.Sighting) bool {
					return matchesInput(fullInput)(s) && s.Credibility == "high"
				})).Return(nil)
			},
			expected: expected{err: nil},
		},
		{
			name: "успешное создание наблюдения (medium credibility)",
			args: args{in: mediumInput},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Create(ctx, mock.MatchedBy(func(s model.Sighting) bool {
					return matchesInput(mediumInput)(s) && s.Credibility == "medium"
				})).Return(nil)
			},
			expected: expected{err: nil},
		},
		{
			name: "успешное создание наблюдения (low credibility)",
			args: args{in: lowInput},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Create(ctx, mock.MatchedBy(func(s model.Sighting) bool {
					return matchesInput(lowInput)(s) && s.Credibility == "low"
				})).Return(nil)
			},
			expected: expected{err: nil},
		},
		{
			name: "ошибка репозитория при создании",
			args: args{in: fullInput},
			setupMock: func(repo *mocks.UFORepository) {
				repo.EXPECT().Create(ctx, mock.MatchedBy(matchesInput(fullInput))).Return(repoErr)
			},
			expected: expected{err: repoErr},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Каждый подтест создаёт СВОЙ мок — изоляция, без data race
			ufoRepository := mocks.NewUFORepository(t)
			tc.setupMock(ufoRepository)

			svc := NewService(ufoRepository)
			uuid, err := svc.Create(ctx, tc.args.in)

			if tc.expected.err != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expected.err)
				assert.Empty(t, uuid)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, uuid)
			}
		})
	}
}

// equalTimePtr возвращает true, если оба указателя nil или оба указывают на равное значение
func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// equalStringPtr возвращает true, если оба указателя nil или оба указывают на равное значение
func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// equalBoolPtr возвращает true, если оба указателя nil или оба указывают на равное значение
func equalBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// equalInt32Ptr возвращает true, если оба указателя nil или оба указывают на равное значение
func equalInt32Ptr(a, b *int32) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
