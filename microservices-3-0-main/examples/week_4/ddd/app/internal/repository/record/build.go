package record

import "time"

// BuildRecord — плоская структура для маппинга строки таблицы pc_builds
type BuildRecord struct {
	UUID      string     `db:"uuid"`
	Status    string     `db:"status"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}
