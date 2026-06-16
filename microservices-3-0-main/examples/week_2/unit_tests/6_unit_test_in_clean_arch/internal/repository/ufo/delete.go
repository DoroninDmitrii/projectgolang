package ufo

import (
	"context"
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/errors"
)

func (r *repository) Delete(_ context.Context, uuid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sighting, ok := r.data[uuid]
	if !ok {
		return errs.ErrSightingNotFound
	}

	// Мягкое удаление - устанавливаем deleted_at
	sighting.DeletedAt = new(time.Now())

	r.data[uuid] = sighting

	return nil
}
