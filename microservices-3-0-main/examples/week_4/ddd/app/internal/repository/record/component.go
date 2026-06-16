package record

import "time"

// ComponentRecord — плоская структура для маппинга строки таблицы components
type ComponentRecord struct {
	UUID          string     `db:"uuid"`
	Name          string     `db:"name"`
	Type          string     `db:"type"`
	Properties    []byte     `db:"properties"`
	StockQuantity int        `db:"stock_quantity"`
	Reserved      int        `db:"reserved"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     *time.Time `db:"updated_at"`
}
