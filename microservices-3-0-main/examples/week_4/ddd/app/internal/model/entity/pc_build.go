package entity

import (
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// PCBuild — экспортируемый алиас для приватной структуры
type PCBuild = pcBuild

// pcBuild — агрегат сборки ПК с инвариантом:
//   - отменённая сборка не может быть отменена повторно
//
// Приватная структура + публичный алиас: тип доступен из других пакетов,
// но создать entity.PCBuild{} извне нельзя — только через RestorePCBuild
// Все поля приватные — защита от прямой модификации
type pcBuild struct {
	uuid      string
	status    valueobject.BuildStatus
	createdAt time.Time
	updatedAt *time.Time
}

// RestorePCBuild восстанавливает сборку из данных хранилища
func RestorePCBuild(
	id string,
	status valueobject.BuildStatus,
	createdAt time.Time,
	updatedAt *time.Time,
) PCBuild {
	return pcBuild{
		uuid:      id,
		status:    status,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}
}

// Cancel отменяет сборку
// Возвращает ErrBuildAlreadyCancelled, если сборка уже отменена
func (b *pcBuild) Cancel() error {
	if b.status == valueobject.BuildStatusCancelled {
		return errs.ErrBuildAlreadyCancelled
	}

	b.status = valueobject.BuildStatusCancelled

	return nil
}

// UUID возвращает уникальный идентификатор сборки
func (b *pcBuild) UUID() string { return b.uuid }

// Status возвращает текущий статус сборки
func (b *pcBuild) Status() valueobject.BuildStatus { return b.status }

// CreatedAt возвращает время создания сборки
func (b *pcBuild) CreatedAt() time.Time { return b.createdAt }
