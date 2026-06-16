package ufo

type service struct {
	ufoRepository      UFORepository
	ufoProducerService UFOProducerService
}

func NewService(ufoRepository UFORepository, ufoProducerService UFOProducerService) *service {
	return &service{
		ufoRepository:      ufoRepository,
		ufoProducerService: ufoProducerService,
	}
}
