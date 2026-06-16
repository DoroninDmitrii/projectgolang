package converter

import (
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/repository/record"
)

func SightingToRepoModel(s model.Sighting) record.Sighting {
	return record.Sighting{
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

func SightingToModel(s record.Sighting) model.Sighting {
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
