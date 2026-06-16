package ufo

import (
	"context"
	"log/slog"
)

type service struct {
	ufoRecordedConsumer Consumer
}

func NewService(ufoRecordedConsumer Consumer) *service {
	return &service{
		ufoRecordedConsumer: ufoRecordedConsumer,
	}
}

func (s *service) RunConsumer(ctx context.Context) error {
	slog.InfoContext(ctx, "запуск потребителя UFORecorded")

	return s.ufoRecordedConsumer.Consume(ctx, s.UFORecordedHandler)
}
