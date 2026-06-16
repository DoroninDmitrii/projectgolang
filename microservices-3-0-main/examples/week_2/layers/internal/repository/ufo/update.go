package ufo

import (
	"context"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/repository/converter"
)

func (r *repository) Update(_ context.Context, sighting model.Sighting) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.data[sighting.Uuid]
	if !ok {
		return errs.ErrSightingNotFound
	}

	if existing.DeletedAt != nil {
		return errs.ErrSightingNotFound
	}

	r.data[sighting.Uuid] = repoConverter.SightingToRepoModel(sighting)

	return nil
}
