package pc_builder

// Handler — обработчик API-запросов сборки ПК
type Handler struct {
	service PCBuilderService
}

// NewHandler создаёт обработчик API сборки ПК
func NewHandler(service PCBuilderService) *Handler {
	return &Handler{
		service: service,
	}
}
