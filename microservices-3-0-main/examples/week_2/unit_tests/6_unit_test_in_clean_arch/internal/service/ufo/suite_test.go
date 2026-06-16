package ufo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/service/ufo/mocks"
)

// ServiceSuite — набор тестов сервисного слоя
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
// В сервисном слое используем Assert (s.NoError, s.Equal — Assert-режим по умолчанию),
// потому что проверки часто независимы друг от друга: можно одновременно проверить
// и ошибку, и пустоту результата
//
// Сравни с API-слоем (suite_test.go), где используется s.Require() — там после проверки
// ошибки мы обращаемся к полям ответа, и при nil-ответе произойдёт panic
//
// Примечание: s.NoError(err) эквивалентно s.Assert().NoError(err)
type ServiceSuite struct {
	suite.Suite

	ctx context.Context

	ufoRepository *mocks.UFORepository

	service *service
}

func (s *ServiceSuite) SetupTest() {
	s.ctx = context.Background()

	s.ufoRepository = mocks.NewUFORepository(s.T())

	s.service = NewService(
		s.ufoRepository,
	)
}

func (s *ServiceSuite) TearDownTest() {
	s.T().Log("TearDownTest: очистка после", s.T().Name())

	// AssertExpectations проверяет, что все ожидаемые вызовы мока действительно произошли
	// Если в тесте мы настроили mock.EXPECT().Get(...).Return(...), но сам метод Get так и не был вызван —
	// AssertExpectations зафейлит тест. Это защищает от ситуации, когда тест "зелёный",
	// но на самом деле тестируемый код не вызывает нужную зависимость
	//
	// Примечание: mockery v3 автоматически вызывает AssertExpectations через t.Cleanup(),
	// поэтому явный вызов здесь избыточен. Но мы оставляем его для наглядности —
	// чтобы показать, как TearDownTest можно использовать для проверки mock-ожиданий
	s.ufoRepository.AssertExpectations(s.T())
}

func TestServiceSuite(t *testing.T) {
	suite.Run(t, new(ServiceSuite))
}
