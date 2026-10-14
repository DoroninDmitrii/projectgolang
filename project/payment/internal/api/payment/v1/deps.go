package v1

import (
	"context"

	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/model"
)

// PaymentService описывает бизнес-логику, необходимую API оплаты.
type PaymentService interface {
	Pay(ctx context.Context, req model.PayRequest) (string, error)
}
