package record

import "time"

type Sighting struct {
	UUID            string
	ObservedAt      *time.Time
	Location        string
	Description     string
	Color           *string
	Sound           *bool
	DurationSeconds *int32
	Credibility     string
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	DeletedAt       *time.Time
}
