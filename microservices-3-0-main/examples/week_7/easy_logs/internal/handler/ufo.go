// Package handler содержит gRPC обработчики
package handler

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/pkg/proto/ufo/v1"
)

// UFOServer реализация gRPC сервера наблюдений НЛО
type UFOServer struct {
	ufov1.UnimplementedUFOServiceServer
}

// NewUFOServer создаёт новый экземпляр UFOServer
func NewUFOServer() *UFOServer {
	return &UFOServer{}
}

// Get обрабатывает запрос на получение наблюдения
func (s *UFOServer) Get(_ context.Context, req *ufov1.GetRequest) (*ufov1.GetResponse, error) {
	if req.GetUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "uuid пуст")
	}

	time.Sleep(time.Duration(rand.IntN(1000)) * time.Millisecond) //nolint:forbidigo,gosec // симуляция задержки

	return &ufov1.GetResponse{
		Sighting: &ufov1.Sighting{
			Uuid: req.GetUuid(),
			Info: &ufov1.SightingInfo{
				Location:    gofakeit.City(),
				Description: gofakeit.Sentence(10),
				Observer:    gofakeit.Name(),
				IsConfirmed: gofakeit.Bool(),
			},
			CreatedAt: timestamppb.New(gofakeit.Date()),
			UpdatedAt: timestamppb.New(gofakeit.Date()),
		},
	}, nil
}
