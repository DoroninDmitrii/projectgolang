package v1

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/converter"
	errs "github.com/DoroninDmitrii/projectgolang/project/payment/internal/errors"
	paymentv1 "github.com/DoroninDmitrii/projectgolang/project/shared/pkg/proto/payment/v1"
)

// PayOrder обрабатывает gRPC-запрос оплаты заказа.
func (a *api) PayOrder(
	ctx context.Context,
	req *paymentv1.PayOrderRequest,
) (*paymentv1.PayOrderResponse, error) {
	transactionUUID, err := a.paymentService.Pay(
		ctx,
		converter.PayOrderRequestToModel(req),
	)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrInvalidOrderUUID):
			return nil, status.Error(
				codes.InvalidArgument,
				errs.ErrInvalidOrderUUID.Error(),
			)
		case errors.Is(err, errs.ErrInvalidPaymentMethod):
			return nil, status.Error(
				codes.InvalidArgument,
				errs.ErrInvalidPaymentMethod.Error(),
			)
		default:
			slog.ErrorContext(ctx, "ошибка оплаты заказа", "error", err)

			return nil, status.Error(codes.Internal, "внутренняя ошибка")
		}
	}

	return &paymentv1.PayOrderResponse{
		TransactionUuid: transactionUUID,
	}, nil
}
