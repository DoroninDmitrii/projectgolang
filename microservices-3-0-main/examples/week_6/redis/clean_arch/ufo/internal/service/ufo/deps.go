package ufo

import (
	"context"
	"time"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/redis/clean_arch/ufo/internal/service/input"
)

// UFORepository определяет контракт для работы с хранилищем наблюдений НЛО
type UFORepository interface {
	Create(ctx context.Context, sighting model.Sighting) error
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}

// UFOCacheRepository определяет контракт для работы с кешем наблюдений НЛО
type UFOCacheRepository interface {
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Set(ctx context.Context, uuid string, sighting model.Sighting, ttl time.Duration) error
	Delete(ctx context.Context, uuid string) error
}
