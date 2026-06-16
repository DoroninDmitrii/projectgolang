package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/auth"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/proto/ufo/v1"
)

const (
	// Таймаут для gRPC вызовов
	grpcCallTimeout = 5 * time.Second
)

// Handler содержит gRPC клиент как зависимость
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
	CreatedBy   string `json:"created_by,omitempty"`
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

// CreateSighting создаёт новое наблюдение
// POST /api/v1/sightings
func (h *Handler) CreateSighting(w http.ResponseWriter, r *http.Request) {
	var req CreateSightingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "некорректный JSON")
		return
	}

	ctx := h.withGRPCAuth(r.Context())

	ctx, cancel := context.WithTimeout(ctx, grpcCallTimeout)
	defer cancel()

	resp, err := h.ufoClient.Create(ctx, &ufov1.CreateRequest{
		Location:    req.Location,
		Description: req.Description,
	})
	if err != nil {
		handleGRPCError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toSightingResponse(resp.GetSighting()))
}

// ListSightings возвращает список всех наблюдений
// GET /api/v1/sightings
func (h *Handler) ListSightings(w http.ResponseWriter, r *http.Request) {
	ctx := h.withGRPCAuth(r.Context())

	ctx, cancel := context.WithTimeout(ctx, grpcCallTimeout)
	defer cancel()

	resp, err := h.ufoClient.List(ctx, &ufov1.ListRequest{})
	if err != nil {
		handleGRPCError(w, err)
		return
	}

	sightings := make([]SightingResponse, 0, len(resp.GetSightings()))
	for _, s := range resp.GetSightings() {
		sightings = append(sightings, toSightingResponse(s))
	}

	writeJSON(w, http.StatusOK, sightings)
}

// GetSighting возвращает наблюдение по ID
// GET /api/v1/sightings/{id}.
func (h *Handler) GetSighting(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "id обязателен")
		return
	}

	ctx := h.withGRPCAuth(r.Context())

	ctx, cancel := context.WithTimeout(ctx, grpcCallTimeout)
	defer cancel()

	resp, err := h.ufoClient.Get(ctx, &ufov1.GetRequest{Id: id})
	if err != nil {
		handleGRPCError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toSightingResponse(resp.GetSighting()))
}

// withGRPCAuth пробрасывает session-token из HTTP контекста в gRPC metadata
// HTTP middleware положил токен в контекст, а эта функция передаёт его в gRPC
func (h *Handler) withGRPCAuth(ctx context.Context) context.Context {
	token, ok := auth.TokenFromContext(ctx)
	if !ok {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, auth.SessionTokenKey, token)
}

func toSightingResponse(s *ufov1.Sighting) SightingResponse {
	return SightingResponse{
		ID:          s.GetId(),
		Location:    s.GetLocation(),
		Description: s.GetDescription(),
		ObservedAt:  s.GetObservedAt().AsTime().Format(time.RFC3339),
		CreatedBy:   s.GetCreatedBy(),
	}
}

// handleGRPCError конвертирует gRPC ошибки в HTTP ответы
func handleGRPCError(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		slog.Error("неизвестная ошибка", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "внутренняя ошибка сервера")
		return
	}

	switch st.Code() {
	case codes.NotFound:
		writeError(w, http.StatusNotFound, "not_found", st.Message())
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, "bad_request", st.Message())
	case codes.Unauthenticated:
		writeError(w, http.StatusUnauthorized, "unauthenticated", st.Message())
	case codes.DeadlineExceeded:
		writeError(w, http.StatusGatewayTimeout, "timeout", "таймаут запроса")
	case codes.Unavailable:
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "бэкенд сервис недоступен")
	case codes.Canceled:
		slog.Info("запрос отменён", "message", st.Message())
		http.Error(w, "запрос отменён", http.StatusServiceUnavailable)
	default:
		slog.Error("ошибка gRPC", "code", st.Code(), "message", st.Message())
		writeError(w, http.StatusInternalServerError, "internal_error", "внутренняя ошибка сервера")
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("ошибка кодирования JSON", "error", err)
	}
}

func writeError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	writeJSON(w, statusCode, ErrorResponse{
		Error:   errorCode,
		Message: message,
	})
}
