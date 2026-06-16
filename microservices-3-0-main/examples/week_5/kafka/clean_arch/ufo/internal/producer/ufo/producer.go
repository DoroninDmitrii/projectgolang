package ufo

import (
	"context"
	"log/slog"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/platform/pkg/kafka"
	eventsv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/shared/pkg/proto/events/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/kafka/clean_arch/ufo/internal/model"
)

type service struct {
	ufoRecordedProducer KafkaProducer
}

func NewService(ufoRecordedProducer KafkaProducer) *service {
	return &service{
		ufoRecordedProducer: ufoRecordedProducer,
	}
}

func (p *service) ProduceUFORecorded(ctx context.Context, event model.UFORecordedEvent) error {
	var observedAt *timestamppb.Timestamp
	if event.ObservedAt != nil {
		observedAt = timestamppb.New(*event.ObservedAt)
	}

	msg := &eventsv1.UFORecorded{
		ObservedAt:  observedAt,
		Location:    event.Location,
		Description: event.Description,
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		slog.ErrorContext(ctx, "не удалось сериализовать UFORecorded", "error", err)
		return err
	}

	return p.ufoRecordedProducer.Send(ctx, &kafka.Message{
		Key:   []byte(event.UUID),
		Value: payload,
	})
}
