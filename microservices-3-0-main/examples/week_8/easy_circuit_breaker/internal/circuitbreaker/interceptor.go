// Package circuitbreaker реализует gRPC client interceptor с паттерном Circuit Breaker
//
// Готового gRPC circuit breaker interceptor'а в экосистеме Go нет:
//   - grpc-ecosystem/go-grpc-middleware v2 включает retry, rate limiter, auth — но не circuit breaker
//   - sony/gobreaker — чистая реализация паттерна, без привязки к gRPC или HTTP
//
// Поэтому обёртку пишем сами: оборачиваем вызов invoker в cb.Execute(),
// а ключевая задача — правильно классифицировать ошибки (бизнес vs инфраструктура)
package circuitbreaker

import (
	"context"
	"errors"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// isBusinessError определяет, является ли ошибка бизнесовой
//
// Зачем это нужно: circuit breaker должен реагировать только на инфраструктурные сбои
// (сервер упал, таймаут, сеть недоступна). Бизнес-ошибки — это нормальная работа сервиса:
// клиент запросил несуществующий ресурс, прислал невалидные данные и т.д
// Если считать их сбоями, circuit breaker будет открываться от обычных 404/400 ответов
func isBusinessError(err error) bool {
	if nil == err {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.NotFound, codes.InvalidArgument, codes.AlreadyExists, codes.PermissionDenied:
		return true
	default:
		return false
	}
}

// NewClientInterceptor создаёт gRPC UnaryClientInterceptor с Circuit Breaker
//
// Interceptor встраивается в цепочку вызовов gRPC клиента (grpc.WithUnaryInterceptor)
// и оборачивает каждый исходящий запрос в cb.Execute(). Это значит, что circuit breaker
// принимает решение ДО отправки запроса на сервер:
//
//   - Closed: запрос отправляется на сервер, результат учитывается в счётчиках
//   - Open: запрос НЕ отправляется, мгновенно возвращается ошибка Unavailable
//   - HalfOpen: пропускается ограниченное число пробных запросов
//
// cb — заранее сконфигурированный Circuit Breaker, который передаётся извне
// Это позволяет вызывающему коду настроить пороги, таймауты и callbacks
func NewClientInterceptor(cb *gobreaker.CircuitBreaker[any]) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// originalErr хранит бизнес-ошибку от сервера
		// Мы не можем вернуть её напрямую из cb.Execute — если вернуть error,
		// gobreaker засчитает это как сбой. Поэтому сохраняем в замыкании,
		// а из cb.Execute возвращаем nil (успех для circuit breaker)
		var originalErr error

		_, cbErr := cb.Execute(func() (any, error) {
			// invoker — это собственно отправка gRPC запроса на сервер
			err := invoker(ctx, method, req, reply, cc, opts...)

			// Бизнес-ошибка: сохраняем для вызывающего кода, но для circuit breaker
			// это успех (return nil) — счётчик ошибок не увеличивается
			if isBusinessError(err) {
				originalErr = err
				return nil, nil
			}

			// Инфраструктурная ошибка (Internal, Unavailable, DeadlineExceeded и др.)
			// или nil (успех) — передаём как есть в gobreaker для подсчёта
			return nil, err
		})

		// Приоритет возврата:
		// 1. Бизнес-ошибка — возвращаем оригинальный gRPC статус клиенту
		if originalErr != nil {
			return originalErr
		}

		// 2. Ошибка от circuit breaker — либо CB открыт и не пустил запрос,
		//    либо это оригинальная инфраструктурная ошибка от сервера
		if cbErr != nil {
			// ErrOpenState — CB в состоянии Open, запрос не отправлялся
			// ErrTooManyRequests — CB в HalfOpen, лимит пробных запросов исчерпан
			// В обоих случаях оборачиваем в gRPC Unavailable, чтобы клиент видел
			// тот же статус-код, который сервер вернул бы при недоступности
			if errors.Is(cbErr, gobreaker.ErrOpenState) || errors.Is(cbErr, gobreaker.ErrTooManyRequests) {
				return status.Errorf(codes.Unavailable, "circuit breaker открыт: %v", cbErr)
			}

			// Оригинальная ошибка от invoker (Internal, DeadlineExceeded и т.д.) —
			// возвращаем как есть, чтобы клиент видел настоящий gRPC статус
			return cbErr
		}

		// 3. Успех — запрос прошёл, ответ уже записан в reply
		return nil
	}
}
