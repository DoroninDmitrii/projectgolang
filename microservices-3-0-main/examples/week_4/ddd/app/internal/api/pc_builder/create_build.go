package pc_builder

import (
	"net/http"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/api/dto"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/api/httputil"
)

// CreateBuild обрабатывает HTTP-запрос на создание сборки ПК
func (h *Handler) CreateBuild(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateBuildRequest
	if !httputil.ReadJSON(w, r, &req) {
		return
	}

	buildUUID, status, err := h.service.CreateBuild(r.Context(), req.ComponentUUIDs)
	if err != nil {
		httputil.WriteError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, &dto.CreateBuildResponse{
		BuildUUID: buildUUID,
		Status:    string(status),
	})
}
