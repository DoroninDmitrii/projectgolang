package model

import "time"

// SightingPGView — плоская модель для автоматического маппинга колонок PostgreSQL
// через pgx.RowToStructByName. Имена тегов `db` соответствуют колонкам таблицы sightings
type SightingPGView struct {
	UUID            string     `db:"uuid"`
	ObservedAt      *time.Time `db:"observed_at"`
	Location        string     `db:"location"`
	Description     string     `db:"description"`
	Color           *string    `db:"color"`
	Sound           *bool      `db:"sound"`
	DurationSeconds *int32     `db:"duration_seconds"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       *time.Time `db:"updated_at"`
	DeletedAt       *time.Time `db:"deleted_at"`
}
