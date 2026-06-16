package model

import (
	"time"
)

// Sighting — доменная модель наблюдения НЛО
// Все поля наблюдения лежат прямо на структуре, без вложенного «Info».
// Структура для входа use case'ов живёт отдельно: см. internal/service/input.
type Sighting struct {
	Uuid            string
	ObservedAt      *time.Time
	Location        string
	Description     string
	Color           *string
	Sound           *bool
	DurationSeconds *int32
	Weather         *WeatherConditions
	Credibility     string
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	DeletedAt       *time.Time
}
