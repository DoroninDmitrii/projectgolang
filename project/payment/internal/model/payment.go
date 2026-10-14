// Package model содержит доменные модели PaymentService
package model

// PaymentMethod — способ оплаты заказа.
// Доменный тип: service-слой работает с ним, а не с proto-enum paymentv1.PaymentMethod
type PaymentMethod string

const (
	PaymentMethodUnspecified   PaymentMethod = "UNSPECIFIED"
	PaymentMethodCard          PaymentMethod = "CARD"
	PaymentMethodSBP           PaymentMethod = "SBP"
	PaymentMethodCreditCard    PaymentMethod = "CREDIT_CARD"
	PaymentMethodInvestorMoney PaymentMethod = "INVESTOR_MONEY"
)

// IsValid сообщает, можно ли провести оплату этим способом
func (m PaymentMethod) IsValid() bool {
	switch m {
	case PaymentMethodCard, PaymentMethodSBP, PaymentMethodCreditCard, PaymentMethodInvestorMoney:
		return true
	default:
		return false
	}
}

// PayRequest — запрос на оплату заказа
type PayRequest struct {
	OrderUUID     string
	PaymentMethod PaymentMethod
}
