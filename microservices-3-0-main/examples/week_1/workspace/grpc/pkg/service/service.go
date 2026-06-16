package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/shared/pkg/proto/ufo/v1"
)

// Service реализует gRPC сервис для работы с наблюдениями НЛО
type Service struct {
	ufov1.UnimplementedUFOServiceServer

	mu        sync.RWMutex
	sightings map[string]*ufov1.Sighting
}

// New создает новый экземпляр сервиса для работы с наблюдениями НЛО
func New() *Service {
	return &Service{
		sightings: make(map[string]*ufov1.Sighting),
	}
}

// Create создает новое наблюдение НЛО
func (s *Service) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	if req.GetInfo() == nil {
		return nil, status.Error(codes.InvalidArgument, "info не может быть nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Генерируем UUID для нового наблюдения
	newUUID := uuid.NewString()

	sighting := &ufov1.Sighting{
		Uuid:      newUUID,
		Info:      req.GetInfo(),
		CreatedAt: timestamppb.New(time.Now()),
	}

	s.sightings[newUUID] = sighting

	slog.InfoContext(ctx, "создано наблюдение", "uuid", newUUID)

	return &ufov1.CreateResponse{
		Uuid: newUUID,
	}, nil
}

// Get возвращает наблюдение НЛО по UUID
func (s *Service) Get(_ context.Context, req *ufov1.GetRequest) (*ufov1.GetResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sighting, ok := s.sightings[req.GetUuid()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	return &ufov1.GetResponse{
		Sighting: proto.Clone(sighting).(*ufov1.Sighting),
	}, nil
}

// Update обновляет существующее наблюдение НЛО
func (s *Service) Update(_ context.Context, req *ufov1.UpdateRequest) (*ufov1.UpdateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sighting, ok := s.sightings[req.GetUuid()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	if req.UpdateInfo == nil {
		return nil, status.Error(codes.InvalidArgument, "update_info не может быть nil")
	}

	// Обновляем поля, только если они были установлены в запросе
	if req.GetUpdateInfo().ObservedAt != nil {
		sighting.Info.ObservedAt = req.GetUpdateInfo().ObservedAt
	}

	if req.GetUpdateInfo().Location != nil {
		sighting.Info.Location = req.GetUpdateInfo().Location.Value
	}

	if req.GetUpdateInfo().Description != nil {
		sighting.Info.Description = req.GetUpdateInfo().Description.Value
	}

	if req.GetUpdateInfo().Color != nil {
		sighting.Info.Color = req.GetUpdateInfo().Color
	}

	if req.GetUpdateInfo().Sound != nil {
		sighting.Info.Sound = req.GetUpdateInfo().Sound
	}

	if req.GetUpdateInfo().DurationSeconds != nil {
		sighting.Info.DurationSeconds = req.GetUpdateInfo().DurationSeconds
	}

	sighting.UpdatedAt = timestamppb.New(time.Now())

	return &ufov1.UpdateResponse{}, nil
}

// Delete удаляет наблюдение НЛО (мягкое удаление - устанавливает deleted_at)
func (s *Service) Delete(_ context.Context, req *ufov1.DeleteRequest) (*ufov1.DeleteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sighting, ok := s.sightings[req.GetUuid()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	// Мягкое удаление - устанавливаем deleted_at (идемпотентно: не перезаписываем если уже удалено)
	if sighting.DeletedAt == nil {
		sighting.DeletedAt = timestamppb.New(time.Now())
	}

	return &ufov1.DeleteResponse{}, nil
}
