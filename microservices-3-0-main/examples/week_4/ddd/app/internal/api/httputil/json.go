package httputil

import (
	"errors"
	"log/slog"
	"net/http"

	json "github.com/goccy/go-json"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
)

// ReadJSON декодирует JSON из тела запроса в dst
// При ошибке отправляет 400 и возвращает false
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}

	return true
}

// WriteJSON сериализует v в JSON и отправляет с указанным статусом
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("ошибка записи JSON-ответа", "error", err)
	}
}

// WriteError маппит доменные ошибки на HTTP-статусы и отправляет JSON-ответ
func WriteError(w http.ResponseWriter, err error) {
	status := mapErrorToStatus(err)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if encErr := json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	}); encErr != nil {
		slog.Error("ошибка записи JSON-ответа об ошибке", "error", encErr)
	}
}

// mapErrorToStatus определяет HTTP-статус по типу доменной ошибки
func mapErrorToStatus(err error) int {
	switch {
	case errors.Is(err, errs.ErrComponentNotFound),
		errors.Is(err, errs.ErrBuildNotFound):
		return http.StatusNotFound

	case errors.Is(err, errs.ErrOutOfStock),
		errors.Is(err, errs.ErrIncompatibleSocket),
		errors.Is(err, errs.ErrIncompatibleRAMType),
		errors.Is(err, errs.ErrIncompatibleTDP),
		errors.Is(err, errs.ErrMotherboardRequired),
		errors.Is(err, errs.ErrBuildAlreadyCancelled),
		errors.Is(err, errs.ErrInvalidProperties):
		return http.StatusUnprocessableEntity

	default:
		slog.Error("необработанная ошибка", "error", err)
		return http.StatusInternalServerError
	}
}
