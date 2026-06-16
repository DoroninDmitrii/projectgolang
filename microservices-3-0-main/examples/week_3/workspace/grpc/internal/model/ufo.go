package model

import "time"

// Sighting — доменная модель наблюдения НЛО
// Все поля наблюдения лежат прямо на структуре, без вложенного «Info».
type Sighting struct {
	Uuid            string
	ObservedAt      *time.Time
	Location        string
	Description     string
	Color           *string
	Sound           *bool
	DurationSeconds *int32
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	DeletedAt       *time.Time
}
