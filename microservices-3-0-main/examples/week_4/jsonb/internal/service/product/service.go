package product

// Service предоставляет бизнес-логику для работы с товарами
type Service struct {
	productRepo ProductRepository
}

// New создаёт сервис товаров
func New(productRepo ProductRepository) *Service {
	return &Service{productRepo: productRepo}
}
