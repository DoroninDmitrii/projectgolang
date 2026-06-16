package ufo

// service предоставляет бизнес-логику для работы с наблюдениями НЛО
type service struct {
	ufoRepo UFORepository
}

// New создаёт сервис наблюдений НЛО
func New(ufoRepo UFORepository) *service {
	return &service{ufoRepo: ufoRepo}
}
