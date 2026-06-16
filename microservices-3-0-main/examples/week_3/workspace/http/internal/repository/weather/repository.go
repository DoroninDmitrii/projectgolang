package weather

import "github.com/jackc/pgx/v5/pgxpool"

// repository — репозиторий для работы с данными о погоде
type repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт новый экземпляр репозитория
func NewRepository(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}
