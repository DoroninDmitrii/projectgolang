package input

import "time"

// CreateSightingInput — вход use case'а создания наблюдения
type CreateSightingInput struct {
	ObservedAt      *time.Time
	Location        string
	Description     string
	Color           *string
	Sound           *bool
	DurationSeconds *int32
}

// UpdateSightingInput — вход use case'а обновления наблюдения (патч-структура)
type UpdateSightingInput struct {
	ObservedAt      *time.Time
	Location        *string
	Description     *string
	Color           *string
	Sound           *bool
	DurationSeconds *int32
}
