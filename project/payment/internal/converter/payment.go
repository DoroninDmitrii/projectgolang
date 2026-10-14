// Package converter преобразует proto-типы PaymentService в доменные модели
package converter

import (
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/model"
	paymentv1 "github.com/DoroninDmitrii/projectgolang/project/shared/pkg/proto/payment/v1"
)

// paymentMethodFromProto сопоставляет proto-enum способа оплаты доменному типу.
// UNSPECIFIED в таблицу не входит: всё, чего здесь нет, становится PaymentMethodUnspecified
var paymentMethodFromProto = map[paymentv1.PaymentMethod]model.PaymentMethod{
	paymentv1.PaymentMethod_PAYMENT_METHOD_CARD:           model.PaymentMethodCard,
	paymentv1.PaymentMethod_PAYMENT_METHOD_SBP:            model.PaymentMethodSBP,
	paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD:    model.PaymentMethodCreditCard,
	paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY: model.PaymentMethodInvestorMoney,
}

// PayOrderRequestToModel преобразует gRPC-запрос оплаты в доменный запрос
func PayOrderRequestToModel(req *paymentv1.PayOrderRequest) model.PayRequest {
	return model.PayRequest{
		OrderUUID:     req.GetOrderUuid(),
		PaymentMethod: PaymentMethodToModel(req.GetPaymentMethod()),
	}
}

// PaymentMethodToModel преобразует proto-enum способа оплаты в доменный тип
func PaymentMethodToModel(method paymentv1.PaymentMethod) model.PaymentMethod {
	m, ok := paymentMethodFromProto[method]
	if !ok {
		return model.PaymentMethodUnspecified
	}

	return m
}
