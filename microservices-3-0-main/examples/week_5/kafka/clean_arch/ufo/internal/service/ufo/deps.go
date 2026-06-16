package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/service/input"
)

// UFORepository определяет контракт для работы с хранилищем наблюдений НЛО
type UFORepository interface {
	Create(ctx context.Context, sighting model.Sighting) error
	Get(ctx context.Context, uuid string) (model.Sighting, error)
	Update(ctx context.Context, uuid string, updateInfo input.UpdateSightingInput) error
	Delete(ctx context.Context, uuid string) error
}

// UFOProducerService определяет контракт для отправки событий о наблюдениях НЛО в Kafka
type UFOProducerService interface {
	ProduceUFORecorded(ctx context.Context, event model.UFORecordedEvent) error
}
