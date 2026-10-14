// Package app собирает зависимости и регистрирует API PaymentService.
package app

import (
	"google.golang.org/grpc"

	paymentAPI "github.com/DoroninDmitrii/projectgolang/project/payment/internal/api/payment/v1"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/interceptor"
	"github.com/DoroninDmitrii/projectgolang/project/payment/internal/service/payment"
	paymentv1 "github.com/DoroninDmitrii/projectgolang/project/shared/pkg/proto/payment/v1"
)

// Interceptors возвращает опции сервера с обработкой непредвиденных ошибок.
func Interceptors() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(interceptor.Error),
	}
}

// RegisterServices собирает сервис оплаты и регистрирует его gRPC API.
func RegisterServices(grpcServer *grpc.Server) {
	svc := payment.New()
	api := paymentAPI.New(svc)

	paymentv1.RegisterPaymentServiceServer(grpcServer, api)
}
