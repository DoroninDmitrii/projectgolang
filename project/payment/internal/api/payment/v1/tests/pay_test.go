package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentAPI "github.com/DoroninDmitrii/projectgolang/project/payment/internal/api/payment/v1"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/api/payment/v1/mocks"
	errs "github.com/DoroninDmitrii/projectgolang/project/payment/internal/errors"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/model"
	paymentv1 "github.com/DoroninDmitrii/projectgolang/project/shared/pkg/proto/payment/v1"
)

func TestPayOrder(t *testing.T) {
	t.Parallel()

	orderUUID := uuid.NewString()
	transactionUUID := uuid.NewString()

	tests := []struct {
		name            string
		transactionUUID string
		serviceErr      error
		wantCode        codes.Code
		wantMessage     string
	}{
		{
			name:            "успешная оплата",
			transactionUUID: transactionUUID,
			wantCode:        codes.OK,
		},
		{
			name:        "ошибка UUID заказа",
			serviceErr:  errs.ErrInvalidOrderUUID,
			wantCode:    codes.InvalidArgument,
			wantMessage: errs.ErrInvalidOrderUUID.Error(),
		},
		{
			name:        "ошибка метода оплаты",
			serviceErr:  errs.ErrInvalidPaymentMethod,
			wantCode:    codes.InvalidArgument,
			wantMessage: errs.ErrInvalidPaymentMethod.Error(),
		},
		{
			name:        "внутренняя ошибка сервиса",
			serviceErr:  errors.New("технические подробности сбоя"),
			wantCode:    codes.Internal,
			wantMessage: "внутренняя ошибка",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			req := &paymentv1.PayOrderRequest{
				OrderUuid:     orderUUID,
				PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
			}

			wantServiceReq := model.PayRequest{
				OrderUUID:     orderUUID,
				PaymentMethod: model.PaymentMethodCard,
			}

			svc := mocks.NewPaymentService(t)
			svc.EXPECT().
				Pay(ctx, wantServiceReq).
				Return(tc.transactionUUID, tc.serviceErr).
				Once()

			api := paymentAPI.New(svc)

			res, err := api.PayOrder(ctx, req)

			if tc.wantCode != codes.OK {
				require.Error(t, err)
				assert.Nil(t, res)

				st, ok := status.FromError(err)
				require.True(t, ok, "ошибка должна быть gRPC-статусом")

				assert.Equal(t, tc.wantCode, st.Code())
				assert.Equal(t, tc.wantMessage, st.Message())
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.transactionUUID, res.GetTransactionUuid())
		})
	}
}
