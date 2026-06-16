package ufo

import (
	"context"
	"log/slog"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka"
)

func (s *service) UFORecordedHandler(ctx context.Context, msg kafka.Message) error {
	event, err := decodeUFORecorded(msg.Value)
	if err != nil {
		slog.ErrorContext(ctx, "не удалось декодировать UFORecorded", "error", err)
		return err
	}

	slog.InfoContext(
		ctx, "обработка сообщения",
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"sighting_uuid", event.UUID,
		"location", event.Location,
		"description", event.Description,
	)

	return nil
}
