package dto

// CreateBuildRequest — запрос на создание сборки ПК
type CreateBuildRequest struct {
	ComponentUUIDs []string `json:"component_uuids"`
}

// CreateBuildResponse — ответ с информацией о созданной сборке
type CreateBuildResponse struct {
	BuildUUID string `json:"build_uuid"`
	Status    string `json:"status"`
}

// CancelBuildRequest — запрос на отмену сборки ПК
type CancelBuildRequest struct {
	BuildUUID string `json:"build_uuid"`
}

// CancelBuildResponse — ответ с информацией об отменённой сборке
type CancelBuildResponse struct {
	BuildUUID string `json:"build_uuid"`
	Status    string `json:"status"`
}
