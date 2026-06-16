package pc_builder

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// TxManager определяет контракт для управления транзакциями
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// CompatibilityChecker определяет контракт для доменного сервиса проверки совместимости
type CompatibilityChecker interface {
	Check(components []entity.Component) error
}

// ComponentRepository определяет контракт для работы с хранилищем комплектующих
type ComponentRepository interface {
	List(ctx context.Context, uuids []string) ([]entity.Component, error)
	ListByBuildUUID(ctx context.Context, buildUUID string) ([]entity.Component, error)
	UpdateReservedBatch(ctx context.Context, components []entity.Component) error
}

// BuildRepository определяет контракт для работы с хранилищем сборок
type BuildRepository interface {
	Create(ctx context.Context, buildUUID string, status valueobject.BuildStatus) error
	AddComponents(ctx context.Context, buildUUID string, componentUUIDs []string) error
	Get(ctx context.Context, uuid string) (entity.PCBuild, error)
	UpdateStatus(ctx context.Context, uuid string, status valueobject.BuildStatus) error
}
