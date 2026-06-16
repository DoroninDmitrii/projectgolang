package model

import "time"

// Account — доменная модель банковского счёта
// Теги `db` используются pgx.RowToStructByName для автоматического маппинга
// колонок результата SQL-запроса на поля структуры по имени
type Account struct {
	UUID        string     `db:"uuid"`
	Owner       string     `db:"owner"`
	Balance     int64      `db:"balance"`     // в копейках
	Description *string    `db:"description"` // nullable
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"` // nullable
}
