package model

import "time"

type Weather struct {
	City        string
	Temperature float64
	UpdatedAt   time.Time
}
