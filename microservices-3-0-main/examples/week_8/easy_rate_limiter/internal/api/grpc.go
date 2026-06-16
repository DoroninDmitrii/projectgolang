package api

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_rate_limiter/pkg/proto/ufo/v1"
)

// CreateUfo создаёт наблюдение (gRPC)
func (a *api) CreateUfo(_ context.Context, req *ufov1.CreateUfoRequest) (*ufov1.CreateUfoResponse, error) {
	if req.GetTitle() == "" {
		return nil, status.Error(codes.InvalidArgument, "заголовок обязателен")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	id := a.nextID
	a.nextID++
	a.sightings[id] = &ufo{ID: id, Title: req.GetTitle(), Content: req.GetContent()}

	slog.Info("наблюдение создано", "id", id)

	return &ufov1.CreateUfoResponse{Id: id}, nil
}

// GetUfo возвращает наблюдение по ID (gRPC)
func (a *api) GetUfo(_ context.Context, req *ufov1.GetUfoRequest) (*ufov1.GetUfoResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	n, exists := a.sightings[req.GetId()]
	if !exists {
		return nil, status.Error(codes.NotFound, "наблюдение не найдено")
	}

	return &ufov1.GetUfoResponse{Id: n.ID, Title: n.Title, Content: n.Content}, nil
}
