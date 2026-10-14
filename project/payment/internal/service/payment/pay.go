package payment

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	errs "github.com/DoroninDmitrii/projectgolang/project/payment/internal/errors"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/model"
)

// Pay проводит оплату заказа и возвращает UUID транзакции
func (s *service) Pay(ctx context.Context, req model.PayRequest) (string, error) {
	// uuid.Parse отклоняет и пустую строку, и строку не в формате UUID
	if _, err := uuid.Parse(req.OrderUUID); err != nil {
		return "", errs.ErrInvalidOrderUUID
	}

	if !req.PaymentMethod.IsValid() {
		return "", errs.ErrInvalidPaymentMethod
	}

	transactionUUID := uuid.NewString()

	slog.InfoContext(ctx, "оплата выполнена",
		"order_uuid", req.OrderUUID,
		"transaction_uuid", transactionUUID,
		"payment_method", req.PaymentMethod,
	)

	return transactionUUID, nil
}
