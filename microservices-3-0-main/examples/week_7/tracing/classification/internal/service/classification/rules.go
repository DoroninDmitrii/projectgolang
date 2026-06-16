package classification

import (
	"strings"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/model"
)

const (
	longObservationThreshold  int32 = 600 // более 10 минут
	shortObservationThreshold int32 = 30  // менее 30 секунд
)

// shapeRule — правило классификации по форме объекта
type shapeRule struct {
	keywords    []string
	objectType  string
	confidence  float32
	explanation string
}

// colorRule — корректировка уверенности по цвету свечения
type colorRule struct {
	colors []string
	bonus  float32
	note   string
}

// shapeRules — правила классификации по ключевым словам в описании
var shapeRules = []shapeRule{
	{
		keywords:    []string{"диск", "disk", "тарелка"},
		objectType:  "classic_saucer",
		confidence:  0.9,
		explanation: "Классическая форма летающей тарелки",
	},
	{
		keywords:    []string{"треугольник", "triangle"},
		objectType:  "triangular_craft",
		confidence:  0.8,
		explanation: "Треугольная форма характерна для современных НЛО",
	},
	{
		keywords:    []string{"сфера", "шар", "sphere"},
		objectType:  "orb",
		confidence:  0.7,
		explanation: "Сферический объект неизвестного происхождения",
	},
}

// colorRules — корректировки уверенности по цвету
var colorRules = []colorRule{
	{colors: []string{"зеленый", "green"}, bonus: 0.1, note: "Зелёное свечение часто наблюдается у НЛО"},
	{colors: []string{"красный", "red"}, bonus: 0.05, note: "Красный цвет может указывать на двигательную систему"},
	{colors: []string{"белый", "white"}, bonus: 0.02, note: "Белое свечение — распространённое явление"},
}

// classifyByShape определяет тип объекта по ключевым словам в описании
func classifyByShape(description string) model.ClassificationResult {
	for _, rule := range shapeRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(description, keyword) {
				return model.ClassificationResult{
					ObjectType:  rule.objectType,
					Confidence:  rule.confidence,
					Explanation: rule.explanation,
				}
			}
		}
	}

	return model.ClassificationResult{
		ObjectType:  "unknown",
		Confidence:  0.5,
		Explanation: "Базовая классификация",
	}
}

// adjustByColor корректирует уверенность на основе цвета свечения
func adjustByColor(result model.ClassificationResult, color string) model.ClassificationResult {
	for _, rule := range colorRules {
		for _, c := range rule.colors {
			if color == c {
				result.Confidence += rule.bonus
				result.Explanation += ". " + rule.note

				return result
			}
		}
	}

	return result
}

// adjustByDuration корректирует уверенность на основе длительности наблюдения
func adjustByDuration(result model.ClassificationResult, durationSeconds int32) model.ClassificationResult {
	switch {
	case durationSeconds > longObservationThreshold:
		result.Confidence += 0.1
		result.Explanation += ". Длительное наблюдение повышает достоверность"
	case durationSeconds < shortObservationThreshold:
		result.Confidence -= 0.1
		result.Explanation += ". Кратковременное наблюдение снижает достоверность"
	}

	return result
}
