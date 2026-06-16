package v1

type api struct {
	weatherRepository WeatherRepository
}

func NewAPI(weatherRepository WeatherRepository) *api {
	return &api{
		weatherRepository: weatherRepository,
	}
}
