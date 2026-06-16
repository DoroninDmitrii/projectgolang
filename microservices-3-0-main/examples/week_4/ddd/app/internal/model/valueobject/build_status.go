package valueobject

import (
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
)

// BuildStatus — статус сборки ПК (перечисление)
type BuildStatus string

const (
	// BuildStatusReserved — сборка зарезервирована
	BuildStatusReserved BuildStatus = "reserved"
	// BuildStatusCancelled — сборка отменена
	BuildStatusCancelled BuildStatus = "cancelled"
)

// NewBuildStatus создаёт BuildStatus с валидацией допустимых значений
func NewBuildStatus(s string) (BuildStatus, error) {
	status := BuildStatus(s)

	switch status {
	case BuildStatusReserved, BuildStatusCancelled:
		return status, nil
	default:
		return "", errs.ErrInvalidBuildStatus
	}
}
