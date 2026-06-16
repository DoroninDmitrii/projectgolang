package record

import "time"

// Product — запись таблицы products.
// Properties здесь — сырые байты JSONB-колонки: репозиторный слой не знает
// о форме доменной структуры. Сериализация/десериализация JSON живёт в
// конвертере, на границе слоёв.
type Product struct {
	ID          string     `db:"id"`
	Name        string     `db:"name"`
	ProductType string     `db:"product_type"`
	Properties  []byte     `db:"properties"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}
