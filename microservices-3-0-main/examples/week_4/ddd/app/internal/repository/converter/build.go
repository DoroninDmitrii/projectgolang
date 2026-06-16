package converter

import (
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/record"
)

// BuildRecordToModel конвертирует запись БД в доменную модель сборки
func BuildRecordToModel(r record.BuildRecord) (entity.PCBuild, error) {
	status, err := valueobject.NewBuildStatus(r.Status)
	if err != nil {
		return entity.PCBuild{}, fmt.Errorf("конвертировать статус сборки: %w", err)
	}

	return entity.RestorePCBuild(
		r.UUID,
		status,
		r.CreatedAt,
		r.UpdatedAt,
	), nil
}
