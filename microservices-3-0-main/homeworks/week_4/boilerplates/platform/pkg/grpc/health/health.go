package health

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Server реализует gRPC Health Checking Protocol (https://github.com/grpc/grpc/blob/master/doc/health-checking.md)
//
// Это стандартный протокол для проверки состояния gRPC-сервисов. Он используется:
//   - Kubernetes: livenessProbe и readinessProbe через grpc-health-probe или нативную gRPC-проверку
//     (spec.containers[].livenessProbe.grpc.port). Kubelet периодически вызывает Check — если сервис
//     не отвечает SERVING, Pod перезапускается (liveness) или исключается из балансировки (readiness)
//   - gRPC-балансировщиками (Envoy, grpc-go client-side balancing) для определения доступных backends
//   - Мониторингом и health-check dashboards
//
// Текущая реализация всегда возвращает SERVING — сервис считается здоровым, если gRPC-сервер
// принимает соединения. Для более продвинутых сценариев можно добавить в структуру зависимости
// (пул БД, Redis-клиент и т.д.) и проверять их доступность в методе Check:
//
//	type Server struct {
//	    grpc_health_v1.UnimplementedHealthServer
//	    db    *pgxpool.Pool   // pool.Ping(ctx) для проверки PostgreSQL
//	    redis *redis.Client   // client.Ping(ctx) для проверки Redis
//	}
//
// Если любая зависимость не отвечает — возвращаем NOT_SERVING, и Kubernetes
// перезапустит Pod (liveness) или уберёт из балансировки (readiness)
type Server struct {
	grpc_health_v1.UnimplementedHealthServer
}

// Check — unary RPC для проверки здоровья сервиса
//
// Вызывается Kubernetes (grpc liveness/readiness probe), балансировщиками и клиентами
// Возвращает один из статусов: SERVING, NOT_SERVING, UNKNOWN
// Поле req.Service позволяет проверить здоровье конкретного сервиса — пустая строка означает
// проверку всего сервера целиком
func (s *Server) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

// Watch — server-streaming RPC для подписки на изменения статуса здоровья
//
// В отличие от Check (pull-модель), Watch позволяет клиенту получать обновления
// в реальном времени без периодического опроса. Используется gRPC-балансировщиками
// для мгновенной реакции на изменение состояния backend'ов
// В текущей реализации отправляет SERVING один раз и завершает стрим
func (s *Server) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	return stream.Send(&grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	})
}

// RegisterService регистрирует health-сервис на gRPC-сервере
// После регистрации сервис доступен по стандартному пути grpc.health.v1.Health
func RegisterService(s *grpc.Server) {
	grpc_health_v1.RegisterHealthServer(s, &Server{})
}
