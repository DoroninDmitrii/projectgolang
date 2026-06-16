package v1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/api/ufo/v1/mocks"
)

// APISuite — набор тестов API-слоя
//
// require vs assert — два пакета testify с разным поведением при провале:
//
//   - require.* (s.Require().NoError и т.д.) вызывает t.FailNow() (→ runtime.Goexit()) —
//     тест ОСТАНАВЛИВАЕТСЯ немедленно. Прерывается только текущий тест/подтест,
//     остальные продолжают работу. Используем для предусловий (gates): если err != nil
//     или res == nil, дальнейшие проверки бессмысленны (будет nil-pointer panic)
//
//   - assert.* (s.NoError, s.Equal и т.д.) вызывает t.Fail() — тест ПРОДОЛЖАЕТСЯ,
//     собирая все ошибки. Используем для проверки конкретных значений (checks):
//     если одно поле неверное, хотим увидеть ВСЕ расхождения, а не только первое
//
// Правило: require — для «ворот» (gates), assert — для «проверок» (checks)
//
// В API-слое используем Require (s.Require()), потому что после проверки ошибки
// мы обращаемся к полям ответа (res.GetSighting().GetUuid()) — если ответ nil, будет panic
//
// Примечание: в testify suite s.NoError(err) — это Assert-режим (эквивалент s.Assert().NoError(err))
// Для Require нужно явно писать s.Require().NoError(err)
type APISuite struct {
	suite.Suite

	ctx context.Context

	ufoService *mocks.UFOService

	api *api
}

func (s *APISuite) SetupTest() {
	s.ctx = context.Background()

	s.ufoService = mocks.NewUFOService(s.T())

	s.api = NewAPI(
		s.ufoService,
	)
}

func (s *APISuite) TearDownTest() {
	s.T().Log("TearDownTest: очистка после", s.T().Name())

	// AssertExpectations проверяет, что все ожидаемые вызовы мока действительно произошли
	// Если в тесте мы настроили mock.EXPECT().Get(...).Return(...), но сам метод Get так и не был вызван —
	// AssertExpectations зафейлит тест. Это защищает от ситуации, когда тест "зелёный",
	// но на самом деле тестируемый код не вызывает нужную зависимость
	//
	// Примечание: mockery v3 автоматически вызывает AssertExpectations через t.Cleanup(),
	// поэтому явный вызов здесь избыточен. Но мы оставляем его для наглядности —
	// чтобы показать, как TearDownTest можно использовать для проверки mock-ожиданий
	s.ufoService.AssertExpectations(s.T())
}

func TestAPISuite(t *testing.T) {
	suite.Run(t, new(APISuite))
}
