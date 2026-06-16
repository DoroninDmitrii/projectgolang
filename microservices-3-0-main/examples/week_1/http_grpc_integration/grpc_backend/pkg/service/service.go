package service

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/shared/pkg/proto/ufo/v1"
)

// Service реализует gRPC сервис для работы с наблюдениями НЛО
type Service struct {
	ufov1.UnimplementedUFOServiceServer

	mu        sync.RWMutex
	sightings map[string]*ufov1.Sighting
}

// New создаёт сервис с начальными данными
func New() *Service {
	now := timestamppb.Now()
	return &Service{
		sightings: map[string]*ufov1.Sighting{
			"sighting-001": {
				Id:          "sighting-001",
				Location:    "Roswell, New Mexico",
				Description: "Яркий диск в небе над пустыней",
				ObservedAt:  now,
			},
			"sighting-002": {
				Id:          "sighting-002",
				Location:    "Phoenix, Arizona",
				Description: "Треугольный объект с огнями",
				ObservedAt:  now,
			},
			"sighting-003": {
				Id:          "sighting-003",
				Location:    "Москва, Россия",
				Description: "Светящийся шар над ВДНХ",
				ObservedAt:  now,
			},
		},
	}
}

// NewEmpty создаёт пустой сервис (для тестов)
func NewEmpty() *Service {
	return &Service{
		sightings: make(map[string]*ufov1.Sighting),
	}
}

// Get возвращает наблюдение по ID
func (s *Service) Get(_ context.Context, req *ufov1.GetRequest) (*ufov1.GetResponse, error) {
	// Валидация
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id обязателен")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sighting, ok := s.sightings[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение %s не найдено", req.GetId())
	}

	return &ufov1.GetResponse{
		Sighting: sighting,
	}, nil
}

// List возвращает все наблюдения
func (s *Service) List(_ context.Context, _ *ufov1.ListRequest) (*ufov1.ListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sightings := make([]*ufov1.Sighting, 0, len(s.sightings))
	for _, sighting := range s.sightings {
		sightings = append(sightings, sighting)
	}

	return &ufov1.ListResponse{
		Sightings: sightings,
	}, nil
}

// Create создаёт новое наблюдение
func (s *Service) Create(_ context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	// Валидация
	if req.GetLocation() == "" {
		return nil, status.Error(codes.InvalidArgument, "location обязателен")
	}
	if req.GetDescription() == "" {
		return nil, status.Error(codes.InvalidArgument, "description обязателен")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sighting := &ufov1.Sighting{
		Id:          uuid.New().String(),
		Location:    req.GetLocation(),
		Description: req.GetDescription(),
		ObservedAt:  timestamppb.Now(),
	}

	s.sightings[sighting.Id] = sighting

	return &ufov1.CreateResponse{
		Sighting: sighting,
	}, nil
}
