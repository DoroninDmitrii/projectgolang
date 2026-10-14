// Package payment содержит бизнес-логику оплаты заказов
package payment

// service реализует бизнес-логику PaymentService.
// Зависимостей нет: сервис stateless — ничего не хранит и никуда не ходит
type service struct{}

// New создаёт сервис оплаты
func New() *service {
	return &service{}
}
