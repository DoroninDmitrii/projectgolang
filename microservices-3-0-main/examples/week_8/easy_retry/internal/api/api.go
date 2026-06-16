package api

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_retry/pkg/proto/ufo/v1"
)

const (
	// failCount — первые N запросов CreateUfo будут возвращать ошибку
	failCount = 2
)

type ufo struct {
	ID      int64
	Title   string
	Content string
}

// api реализует gRPC-обработчик UFOService
type api struct {
	ufov1.UnimplementedUFOServiceServer
	mu             sync.RWMutex
	sightings      map[int64]*ufo
	nextID         int64
	createCalls    atomic.Int64
	lastCreateCall atomic.Int64 // UnixNano последнего вызова CreateUfo
}

// NewAPI создаёт новый gRPC-обработчик UFOService
func NewAPI() *api {
	return &api{
		sightings: make(map[int64]*ufo),
		nextID:    1,
	}
}

// CreateUfo создаёт новое наблюдение. Первые failCount вызовов симулируют сбой
func (a *api) CreateUfo(_ context.Context, req *ufov1.CreateUfoRequest) (*ufov1.CreateUfoResponse, error) {
	// Валидация идёт до симуляции сбоя: бизнес-ошибки должны возвращаться сразу,
	// без транзиентных Unavailable, чтобы демонстрация selective retry была чистой
	if req.GetTitle() == "" {
		slog.Warn("бизнес-ошибка: пустой заголовок (retry клиент применять не должен)")
		return nil, status.Error(codes.InvalidArgument, "заголовок обязателен")
	}

	now := time.Now()
	callNum := a.createCalls.Add(1)

	// Вычисляем время с предыдущего запроса, чтобы наглядно видеть backoff
	var sinceLastCall time.Duration
	if prev := a.lastCreateCall.Swap(now.UnixNano()); prev != 0 {
		sinceLastCall = now.Sub(time.Unix(0, prev))
	}

	if callNum <= int64(failCount) {
		slog.Warn(
			"симуляция сбоя",
			"call", callNum,
			"fail_count", failCount,
			"since_prev", sinceLastCall.Round(time.Millisecond).String(),
		)
		return nil, status.Error(codes.Unavailable, "сервис временно недоступен")
	}

	// Сбрасываем счётчик после успеха, чтобы следующий запуск клиента
	// снова проходил через failCount сбоев
	a.createCalls.Store(0)
	a.lastCreateCall.Store(0)

	a.mu.Lock()
	defer a.mu.Unlock()

	id := a.nextID
	a.nextID++
	a.sightings[id] = &ufo{ID: id, Title: req.GetTitle(), Content: req.GetContent()}

	slog.Info(
		"наблюдение создано",
		"id", id,
		"since_prev", sinceLastCall.Round(time.Millisecond).String(),
	)

	return &ufov1.CreateUfoResponse{Id: id}, nil
}

// GetUfo возвращает наблюдение по идентификатору
func (a *api) GetUfo(_ context.Context, req *ufov1.GetUfoRequest) (*ufov1.GetUfoResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	n, exists := a.sightings[req.GetId()]
	if !exists {
		return nil, status.Error(codes.NotFound, "наблюдение не найдено")
	}

	return &ufov1.GetUfoResponse{Id: n.ID, Title: n.Title, Content: n.Content}, nil
}
