package model

import "time"

// Account — доменная модель банковского счёта
type Account struct {
	UUID        string
	Owner       string
	Balance     int64   // в копейках
	Description *string // nullable
	CreatedAt   time.Time
	UpdatedAt   *time.Time // nullable
}
