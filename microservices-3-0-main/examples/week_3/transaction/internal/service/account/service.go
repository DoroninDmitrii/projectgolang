package account

// Service предоставляет бизнес-логику для работы со счетами
type Service struct {
	txManager   TxManager
	accountRepo AccountRepository
}

// New создаёт сервис счетов
func New(txManager TxManager, accountRepo AccountRepository) *Service {
	return &Service{
		txManager:   txManager,
		accountRepo: accountRepo,
	}
}
