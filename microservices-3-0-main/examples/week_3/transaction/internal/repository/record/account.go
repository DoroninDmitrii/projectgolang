package record

import "time"

// Account — запись таблицы accounts.
// db-теги намечают позицию для сторонних мапперов (squirrel, pgx StructScan);
// текущий пример читает поля по позиции, но record-слой намеренно остаётся
// отдельным от доменной model.Account.
type Account struct {
	UUID        string     `db:"uuid"`
	Owner       string     `db:"owner"`
	Balance     int64      `db:"balance"`
	Description *string    `db:"description"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}
