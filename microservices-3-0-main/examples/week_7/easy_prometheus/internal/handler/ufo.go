// Package handler содержит gRPC обработчики
package handler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/metrics"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/pkg/proto/ufo/v1"
)

// sighting — наблюдение НЛО (in-memory хранилище)
type sighting struct {
	UUID      string
	Info      *ufov1.SightingInfo
	CreatedAt time.Time
}

// UFOServer — реализация gRPC сервиса наблюдений НЛО
type UFOServer struct {
	ufov1.UnimplementedUFOServiceServer
	mu        sync.RWMutex
	sightings map[string]*sighting
}

// NewUFOServer создаёт новый экземпляр UFOServer
func NewUFOServer() *UFOServer {
	return &UFOServer{
		sightings: make(map[string]*sighting),
	}
}

// Create создаёт новое наблюдение НЛО
func (s *UFOServer) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	if req.GetInfo().GetLocation() == "" {
		return nil, status.Error(codes.InvalidArgument, "место наблюдения обязательно")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newUUID := uuid.NewString()
	s.sightings[newUUID] = &sighting{
		UUID:      newUUID,
		Info:      req.GetInfo(),
		CreatedAt: time.Now(),
	}

	// Бизнес-метрики (push через OTel → Collector → Prometheus)
	metrics.SightingsCreatedTotal.Add(ctx, 1) // монотонный счётчик: +1
	metrics.SightingsActive.Add(ctx, 1)       // текущее количество: +1

	slog.Info("наблюдение создано", "uuid", newUUID)

	return &ufov1.CreateResponse{Uuid: newUUID}, nil
}

// Delete удаляет наблюдение НЛО по идентификатору
func (s *UFOServer) Delete(ctx context.Context, req *ufov1.DeleteRequest) (*ufov1.DeleteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sightings[req.GetUuid()]; !exists {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	delete(s.sightings, req.GetUuid())

	metrics.SightingsActive.Add(ctx, -1) // текущее количество: -1

	slog.Info("наблюдение удалено", "uuid", req.GetUuid())

	return &ufov1.DeleteResponse{}, nil
}

// Get возвращает наблюдение НЛО по идентификатору
func (s *UFOServer) Get(_ context.Context, req *ufov1.GetRequest) (*ufov1.GetResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sg, exists := s.sightings[req.GetUuid()]
	if !exists {
		return nil, status.Error(codes.NotFound, "наблюдение не найдено")
	}

	return &ufov1.GetResponse{
		Sighting: &ufov1.Sighting{
			Uuid:      sg.UUID,
			Info:      sg.Info,
			CreatedAt: timestamppb.New(sg.CreatedAt),
		},
	}, nil
}

// List возвращает список всех наблюдений НЛО
func (s *UFOServer) List(_ context.Context, _ *ufov1.ListRequest) (*ufov1.ListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := &ufov1.ListResponse{}
	for _, sg := range s.sightings {
		resp.Sightings = append(resp.Sightings, &ufov1.Sighting{
			Uuid:      sg.UUID,
			Info:      sg.Info,
			CreatedAt: timestamppb.New(sg.CreatedAt),
		})
	}

	return resp, nil
}
