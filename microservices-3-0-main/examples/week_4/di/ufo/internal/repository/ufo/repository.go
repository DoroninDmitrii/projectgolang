package ufo

import "github.com/jackc/pgx/v5/pgxpool"

// repository предоставляет доступ к данным о наблюдениях НЛО в PostgreSQL
type repository struct {
	pool *pgxpool.Pool
}

// New создаёт репозиторий наблюдений НЛО
func New(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}
