package weather_center_stub

import "fmt"

// WeatherCenter — стаб погодного центра
// В отличие от мока, стаб возвращает заранее заданные значения без возможности
// проверить, как именно он был вызван. Это ограничение стабов — если нужна
// гибкая настройка поведения и верификация вызовов, используйте моки (пример 5)
type WeatherCenter struct {
	temperatureByCity map[string]float32
}

func NewWeatherCenter() *WeatherCenter {
	return &WeatherCenter{
		temperatureByCity: map[string]float32{
			"Москва":          25.0,
			"Якутск":          -50.0,
			"Сочи":            30.0,
			"Мурманск":        -10.0,
			"Краснодар":       15.0,
			"Астрахань":       40.0,
			"Норильск":        -30.0,
			"Санкт-Петербург": 5.0,
		},
	}
}

func (w *WeatherCenter) SetTemperature(_ string, _ float32) {
	// Стаб игнорирует установку температуры — значения захардкожены
}

func (w *WeatherCenter) GetTemperature(city string) (float32, error) {
	temp, ok := w.temperatureByCity[city]
	if !ok {
		return 0, fmt.Errorf("город %q не найден", city)
	}

	return temp, nil
}
