package tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errs "github.com/DoroninDmitrii/projectgolang/project/payment/internal/errors"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/model"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/service/payment"
)

func TestPay(t *testing.T) {
	t.Parallel()

	orderUUID := uuid.NewString()

	tests := []struct {
		name          string
		orderUUID     string
		paymentMethod model.PaymentMethod
		wantErr       error
	}{
		{
			name:          "оплата картой",
			orderUUID:     orderUUID,
			paymentMethod: model.PaymentMethodCard,
		},
		{
			name:          "оплата через СБП",
			orderUUID:     orderUUID,
			paymentMethod: model.PaymentMethodSBP,
		},
		{
			name:          "оплата кредитной картой",
			orderUUID:     orderUUID,
			paymentMethod: model.PaymentMethodCreditCard,
		},
		{
			name:          "оплата деньгами инвестора",
			orderUUID:     orderUUID,
			paymentMethod: model.PaymentMethodInvestorMoney,
		},
		{
			name:          "невалидный UUID заказа",
			orderUUID:     "not-a-uuid",
			paymentMethod: model.PaymentMethodCard,
			wantErr:       errs.ErrInvalidOrderUUID,
		},
		{
			name:          "пустой UUID заказа",
			orderUUID:     "",
			paymentMethod: model.PaymentMethodCard,
			wantErr:       errs.ErrInvalidOrderUUID,
		},
		{
			name:          "метод оплаты не указан",
			orderUUID:     orderUUID,
			paymentMethod: model.PaymentMethodUnspecified,
			wantErr:       errs.ErrInvalidPaymentMethod,
		},
		{
			name:          "неизвестный метод оплаты",
			orderUUID:     orderUUID,
			paymentMethod: model.PaymentMethod("UNKNOWN"),
			wantErr:       errs.ErrInvalidPaymentMethod,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := payment.New()

			transactionUUID, err := svc.Pay(
				context.Background(),
				model.PayRequest{
					OrderUUID:     tc.orderUUID,
					PaymentMethod: tc.paymentMethod,
				},
			)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, transactionUUID)
				return
			}

			require.NoError(t, err)

			parsedUUID, parseErr := uuid.Parse(transactionUUID)
			require.NoError(t, parseErr)
			assert.Equal(t, uuid.Version(4), parsedUUID.Version())
		})
	}
}
