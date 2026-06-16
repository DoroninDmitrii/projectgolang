package product

import "github.com/jackc/pgx/v5/pgxpool"

type repository struct {
	pool *pgxpool.Pool
}

// New создаёт репозиторий товаров, работающий с PostgreSQL
func New(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}
