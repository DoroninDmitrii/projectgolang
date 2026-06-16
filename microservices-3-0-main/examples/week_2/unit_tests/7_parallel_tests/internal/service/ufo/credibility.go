package ufo

import "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/7_parallel_tests/internal/model"

// calculateCredibility считает score по заполненности 6 полей наблюдения
// и возвращает уровень достоверности: high (≥5), medium (≥3), low (остальное).
func calculateCredibility(s model.Sighting) string {
	score := 0

	if s.ObservedAt != nil {
		score++
	}

	if s.Location != "" {
		score++
	}

	if s.Description != "" {
		score++
	}

	if s.Color != nil {
		score++
	}

	if s.Sound != nil {
		score++
	}

	if s.DurationSeconds != nil {
		score++
	}

	switch {
	case score >= 5:
		return "high"
	case score >= 3:
		return "medium"
	default:
		return "low"
	}
}
