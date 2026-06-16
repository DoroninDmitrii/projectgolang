package ufo

import (
	"context"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/6_unit_test_in_clean_arch/internal/repository/converter"
)

func (r *repository) Get(_ context.Context, uuid string) (model.Sighting, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repoSighting, ok := r.data[uuid]
	if !ok {
		return model.Sighting{}, errs.ErrSightingNotFound
	}

	if repoSighting.DeletedAt != nil {
		return model.Sighting{}, errs.ErrSightingNotFound
	}

	return repoConverter.SightingToModel(repoSighting), nil
}
