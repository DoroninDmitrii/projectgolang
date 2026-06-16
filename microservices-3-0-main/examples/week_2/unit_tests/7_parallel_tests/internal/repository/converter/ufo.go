package converter

import (
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/model"
	repoModel "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/repository/model"
)

func SightingToRepoModel(s model.Sighting) repoModel.Sighting {
	return repoModel.Sighting{
		UUID:            s.Uuid,
		ObservedAt:      s.ObservedAt,
		Location:        s.Location,
		Description:     s.Description,
		Color:           s.Color,
		Sound:           s.Sound,
		DurationSeconds: s.DurationSeconds,
		Credibility:     s.Credibility,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		DeletedAt:       s.DeletedAt,
	}
}

func SightingToModel(s repoModel.Sighting) model.Sighting {
	return model.Sighting{
		Uuid:            s.UUID,
		ObservedAt:      s.ObservedAt,
		Location:        s.Location,
		Description:     s.Description,
		Color:           s.Color,
		Sound:           s.Sound,
		DurationSeconds: s.DurationSeconds,
		Credibility:     s.Credibility,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		DeletedAt:       s.DeletedAt,
	}
}
