package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	weatherCenterStub "github.com/olezhek28-courses/microservices-course-examples/examples/week_2/unit_tests/4_unit_test_with_stubs/weather_center_stub"
)

func TestGetWeatherAdvice(t *testing.T) {
	tests := []struct {
		name     string
		city     string
		expected string
	}{
		{
			name:     "Москва +25 — отличная погода",
			city:     "Москва",
			expected: "Отличная погода для прогулок",
		},
		{
			name:     "Якутск -50 — лучше не выходить",
			city:     "Якутск",
			expected: "Лучше не суйся на улицу",
		},
		{
			name:     "Сочи +30 — жарковато",
			city:     "Сочи",
			expected: "Жарковато, но можно выйти на улицу",
		},
		{
			name:     "Мурманск -10 — прохладно",
			city:     "Мурманск",
			expected: "Прохладно, но можно выйти на улицу",
		},
		{
			name:     "Краснодар +15 — нормальная температура",
			city:     "Краснодар",
			expected: "Температура нормальная, можно гулять",
		},
		{
			name:     "Астрахань +40 — жарко",
			city:     "Астрахань",
			expected: "Жарко, лучше остаться дома",
		},
		{
			name:     "Норильск -30 — одевайся теплее",
			city:     "Норильск",
			expected: "Можно идти гулять, но одевайся теплее",
		},
		{
			name:     "Санкт-Петербург +5 — нормальная температура",
			city:     "Санкт-Петербург",
			expected: "Температура нормальная, можно гулять",
		},
	}

	clientStub := weatherCenterStub.NewWeatherCenter()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := getWeatherAdvice(clientStub, tc.city)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res)
		})
	}

	t.Run("неизвестный город — ошибка стаба", func(t *testing.T) {
		_, err := getWeatherAdvice(clientStub, "НесуществующийГород")
		require.Error(t, err)
	})
}
