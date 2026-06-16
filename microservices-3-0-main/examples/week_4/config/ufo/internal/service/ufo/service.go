package ufo

type service struct {
	ufoRepository UFORepository
}

func NewService(ufoRepository UFORepository) *service {
	return &service{
		ufoRepository: ufoRepository,
	}
}
