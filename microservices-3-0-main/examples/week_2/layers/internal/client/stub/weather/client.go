package weather

import (
	"context"
	"strings"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_2/layers/internal/model"
)

// knownLocations содержит захардкоженные погодные условия для знаменитых UFO-локаций
var knownLocations = map[string]model.WeatherConditions{
	"roswell": {
		Description:        "Ясная ночь, идеальная видимость",
		TemperatureCelsius: 28.5,
		VisibilityKm:       15.0,
	},
	"area 51": {
		Description:        "Сухо, безоблачно, лёгкий ветер",
		TemperatureCelsius: 35.0,
		VisibilityKm:       20.0,
	},
	"tunguska": {
		Description:        "Облачно, низкое давление",
		TemperatureCelsius: -12.0,
		VisibilityKm:       5.0,
	},
	"rendlesham": {
		Description:        "Туман, моросящий дождь",
		TemperatureCelsius: 4.0,
		VisibilityKm:       2.0,
	},
}

// defaultWeather — погода по умолчанию для неизвестных локаций
var defaultWeather = model.WeatherConditions{
	Description:        "Переменная облачность",
	TemperatureCelsius: 18.0,
	VisibilityKm:       10.0,
}

type client struct{}

// New создаёт stub-клиент погоды, который возвращает захардкоженные данные
// Используется для автономной работы сервера без внешних зависимостей
func New() *client {
	return &client{}
}

// GetCurrentWeather возвращает погодные условия для указанной локации
// Для известных UFO-локаций возвращает тематические данные, для остальных — погоду по умолчанию
func (c *client) GetCurrentWeather(_ context.Context, location string) (model.WeatherConditions, error) {
	if w, ok := knownLocations[strings.ToLower(location)]; ok {
		return w, nil
	}

	return defaultWeather, nil
}
