package analysis

// service — сервис для анализа наблюдений НЛО
type service struct {
	ufoClient            UFOClient
	classificationClient ClassificationClient
}

// NewService создаёт новый сервис анализа
func NewService(ufoClient UFOClient, classificationClient ClassificationClient) *service {
	return &service{
		ufoClient:            ufoClient,
		classificationClient: classificationClient,
	}
}
