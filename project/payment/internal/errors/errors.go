// Package errs содержит доменные ошибки PaymentService
package errs

import "errors"

var (
	// ErrInvalidOrderUUID — order_uuid пустой или не в формате UUID
	ErrInvalidOrderUUID = errors.New("неверный формат UUID заказа")

	// ErrInvalidPaymentMethod — способ оплаты не указан или неизвестен
	ErrInvalidPaymentMethod = errors.New("неверный метод оплаты")
)
