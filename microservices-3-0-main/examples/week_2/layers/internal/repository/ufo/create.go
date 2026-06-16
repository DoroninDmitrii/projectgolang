package ufo

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/repository/converter"
)

func (r *repository) Create(_ context.Context, sighting model.Sighting) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[sighting.Uuid] = repoConverter.SightingToRepoModel(sighting)

	return nil
}
