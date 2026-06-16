package model

// Sighting — данные о наблюдении НЛО
type Sighting struct {
	UUID            string
	Description     string
	Color           string
	DurationSeconds int32
}
