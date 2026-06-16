package pc_builder

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/api/dto"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/api/httputil"
)

// CancelBuild обрабатывает HTTP-запрос на отмену сборки ПК
func (h *Handler) CancelBuild(w http.ResponseWriter, r *http.Request) {
	buildUUID := chi.URLParam(r, "uuid")
	if buildUUID == "" {
		http.Error(w, "uuid сборки обязателен", http.StatusBadRequest)
		return
	}

	status, err := h.service.CancelBuild(r.Context(), buildUUID)
	if err != nil {
		httputil.WriteError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, &dto.CancelBuildResponse{
		BuildUUID: buildUUID,
		Status:    string(status),
	})
}
