package ufo

type service struct {
	ufoRepository UFORepository
	weatherClient WeatherClient
}

func NewService(ufoRepository UFORepository, weatherClient WeatherClient) *service {
	return &service{
		ufoRepository: ufoRepository,
		weatherClient: weatherClient,
	}
}
