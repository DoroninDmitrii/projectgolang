package converter

import (
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
	weatherv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/pkg/proto/weather/v1"
)

// WeatherConditionsFromProto конвертирует транспортный DTO погодных условий в доменную модель
func WeatherConditionsFromProto(conditions *weatherv1.WeatherConditions) model.WeatherConditions {
	return model.WeatherConditions{
		Description:        conditions.GetDescription(),
		TemperatureCelsius: conditions.GetTemperatureCelsius(),
		VisibilityKm:       conditions.GetVisibilityKm(),
	}
}
