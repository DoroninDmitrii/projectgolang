package pc_builder

// Service предоставляет бизнес-логику для сборки ПК
type Service struct {
	txManager            TxManager
	componentRepo        ComponentRepository
	buildRepo            BuildRepository
	compatibilityChecker CompatibilityChecker
}

// NewService создаёт сервис сборки ПК
func NewService(
	txManager TxManager,
	componentRepo ComponentRepository,
	buildRepo BuildRepository,
	compatibilityChecker CompatibilityChecker,
) *Service {
	return &Service{
		txManager:            txManager,
		componentRepo:        componentRepo,
		buildRepo:            buildRepo,
		compatibilityChecker: compatibilityChecker,
	}
}
