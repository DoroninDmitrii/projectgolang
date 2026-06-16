package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/shared/pkg/proto/ufo/v1"
)

const (
	// Таймаут для gRPC вызовов
	grpcCallTimeout = 5 * time.Second
)

// Handler содержит gRPC клиент как зависимость
// Это КЛЮЧЕВОЙ ПАТТЕРН: HTTP handler использует gRPC клиент для вызова бэкенда
type Handler struct {
	ufoClient ufov1.UFOServiceClient
}

// New создаёт handler с gRPC клиентом
func New(client ufov1.UFOServiceClient) *Handler {
	return &Handler{ufoClient: client}
}

// SightingResponse представляет наблюдение в HTTP ответе
type SightingResponse struct {
	ID          string `json:"id"`
	Location    string `json:"location"`
	Description string `json:"description"`
	ObservedAt  string `json:"observed_at"`
}

// CreateSightingRequest представляет запрос на создание наблюдения
type CreateSightingRequest struct {
	Location    string `json:"location"`
	Description string `json:"description"`
}

// ErrorResponse представляет ошибку в HTTP ответе
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// ListSightings возвращает список всех наблюдений
// GET /api/v1/sightings
func (h *Handler) ListSightings(w http.ResponseWriter, r *http.Request) {
	// ПАТТЕРН 1: Пробрасывание контекста с таймаутом
	// Всегда устанавливайте таймаут для gRPC вызовов!
	ctx, cancel := context.WithTimeout(r.Context(), grpcCallTimeout)
	defer cancel()

	// Вызов gRPC бэкенда
	resp, err := h.ufoClient.List(ctx, &ufov1.ListRequest{})
	if err != nil {
		// ПАТТЕРН 2: Обработка gRPC ошибок
		handleGRPCError(ctx, w, err)
		return
	}

	// Конвертация proto → HTTP response
	sightings := make([]SightingResponse, 0, len(resp.Sightings))
	for _, s := range resp.Sightings {
		sightings = append(sightings, SightingResponse{
			ID:          s.Id,
			Location:    s.Location,
			Description: s.Description,
			ObservedAt:  s.ObservedAt.AsTime().Format(time.RFC3339),
		})
	}

	writeJSON(ctx, w, http.StatusOK, sightings)
}

// GetSighting возвращает наблюдение по ID
// GET /api/v1/sightings/{id}.
func (h *Handler) GetSighting(w http.ResponseWriter, r *http.Request) {
	// Извлекаем ID из пути
	// /api/v1/sightings/sighting-001 → sighting-001
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sightings/")
	if path == "" {
		writeError(r.Context(), w, http.StatusBadRequest, "bad_request", "id обязателен")
		return
	}

	// ПАТТЕРН 1: Контекст с таймаутом
	ctx, cancel := context.WithTimeout(r.Context(), grpcCallTimeout)
	defer cancel()

	// Вызов gRPC бэкенда
	resp, err := h.ufoClient.Get(ctx, &ufov1.GetRequest{Id: path})
	if err != nil {
		// ПАТТЕРН 2: Обработка gRPC ошибок
		handleGRPCError(ctx, w, err)
		return
	}

	sighting := SightingResponse{
		ID:          resp.Sighting.Id,
		Location:    resp.Sighting.Location,
		Description: resp.Sighting.Description,
		ObservedAt:  resp.Sighting.ObservedAt.AsTime().Format(time.RFC3339),
	}

	writeJSON(ctx, w, http.StatusOK, sighting)
}

// CreateSighting создаёт новое наблюдение
// POST /api/v1/sightings
func (h *Handler) CreateSighting(w http.ResponseWriter, r *http.Request) {
	var req CreateSightingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(r.Context(), w, http.StatusBadRequest, "bad_request", "некорректный JSON")
		return
	}

	// ПАТТЕРН 1: Контекст с таймаутом
	ctx, cancel := context.WithTimeout(r.Context(), grpcCallTimeout)
	defer cancel()

	// Вызов gRPC бэкенда
	resp, err := h.ufoClient.Create(ctx, &ufov1.CreateRequest{
		Location:    req.Location,
		Description: req.Description,
	})
	if err != nil {
		// ПАТТЕРН 2: Обработка gRPC ошибок
		handleGRPCError(ctx, w, err)
		return
	}

	sighting := SightingResponse{
		ID:          resp.Sighting.Id,
		Location:    resp.Sighting.Location,
		Description: resp.Sighting.Description,
		ObservedAt:  resp.Sighting.ObservedAt.AsTime().Format(time.RFC3339),
	}

	writeJSON(ctx, w, http.StatusCreated, sighting)
}

// SetupMux настраивает роутинг для HTTP handler
func SetupMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()

	// Роутинг для /api/v1/sightings
	mux.HandleFunc("/api/v1/sightings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListSightings(w, r)
		case http.MethodPost:
			h.CreateSighting(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Роутинг для /api/v1/sightings/{id}
	mux.HandleFunc("/api/v1/sightings/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			h.GetSighting(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return mux
}

// handleGRPCError конвертирует gRPC ошибки в HTTP ответы
// Это КЛЮЧЕВОЙ ПАТТЕРН: правильная конвертация кодов ошибок
func handleGRPCError(ctx context.Context, w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		// Неизвестная ошибка
		slog.ErrorContext(ctx, "неизвестная ошибка", "error", err)
		writeError(ctx, w, http.StatusInternalServerError, "internal_error", "внутренняя ошибка сервера")
		return
	}

	// Маппинг gRPC кодов → HTTP кодов
	switch st.Code() {
	case codes.NotFound:
		writeError(ctx, w, http.StatusNotFound, "not_found", st.Message())

	case codes.InvalidArgument:
		writeError(ctx, w, http.StatusBadRequest, "bad_request", st.Message())

	case codes.DeadlineExceeded:
		// Таймаут — клиент может повторить запрос
		writeError(ctx, w, http.StatusGatewayTimeout, "timeout", "таймаут запроса")

	case codes.Unavailable:
		// gRPC сервис недоступен
		writeError(ctx, w, http.StatusServiceUnavailable, "service_unavailable", "бэкенд сервис недоступен")

	case codes.Canceled:
		// Запрос был отменён (например, клиент закрыл соединение)
		slog.InfoContext(ctx, "запрос отменён", "message", st.Message())
		http.Error(w, "запрос отменён", http.StatusServiceUnavailable)

	default:
		slog.ErrorContext(ctx, "ошибка gRPC", "code", st.Code(), "message", st.Message())
		writeError(ctx, w, http.StatusInternalServerError, "internal_error", "внутренняя ошибка сервера")
	}
}

func writeJSON(ctx context.Context, w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.ErrorContext(ctx, "ошибка кодирования JSON", "error", err)
	}
}

func writeError(ctx context.Context, w http.ResponseWriter, statusCode int, errorCode, message string) {
	writeJSON(ctx, w, statusCode, ErrorResponse{
		Error:   errorCode,
		Message: message,
	})
}
