package ufo

import "github.com/jackc/pgx/v5/pgxpool"

// repository — репозиторий для работы с наблюдениями НЛО
type repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт новый экземпляр репозитория
func NewRepository(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}
