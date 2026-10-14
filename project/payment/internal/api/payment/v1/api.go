// Package v1 содержит gRPC API оплаты первой версии.
package v1

import (
	paymentv1 "github.com/DoroninDmitrii/projectgolang/project/shared/pkg/proto/payment/v1"
)

type api struct {
	paymentv1.UnimplementedPaymentServiceServer

	paymentService PaymentService
}

// New создаёт API оплаты с переданной бизнес-логикой.
func New(paymentService PaymentService) *api {
	return &api{
		paymentService: paymentService,
	}
}
