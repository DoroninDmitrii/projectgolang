package ufo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/service/input"
)

// Create создаёт наблюдение НЛО и публикует событие UFORecorded в Kafka
//
// ВАЖНО: здесь присутствует проблема dual write — запись в БД и отправка в Kafka не атомарны
// Если Kafka-отправка упадёт, запись в БД останется, но событие не будет доставлено
// В продакшене стоит использовать Transactional Outbox Pattern или Saga
func (s *service) Create(ctx context.Context, in input.CreateSightingInput) (string, error) {
	sighting := model.Sighting{
		Uuid:            uuid.NewString(),
		ObservedAt:      in.ObservedAt,
		Location:        in.Location,
		Description:     in.Description,
		Color:           in.Color,
		Sound:           in.Sound,
		DurationSeconds: in.DurationSeconds,
		CreatedAt:       time.Now(),
	}

	if err := s.ufoRepository.Create(ctx, sighting); err != nil {
		slog.ErrorContext(ctx, "не удалось создать наблюдение", "error", err)
		return "", fmt.Errorf("создать наблюдение: %w", err)
	}

	if err := s.ufoProducerService.ProduceUFORecorded(ctx, model.UFORecordedEvent{
		UUID:        sighting.Uuid,
		ObservedAt:  sighting.ObservedAt,
		Location:    sighting.Location,
		Description: sighting.Description,
	}); err != nil {
		slog.ErrorContext(ctx, "не удалось опубликовать событие UFORecorded", "uuid", sighting.Uuid, "error", err)
		return "", fmt.Errorf("опубликовать UFORecorded: %w", err)
	}

	return sighting.Uuid, nil
}
